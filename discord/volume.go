package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

const VOLUME_USAGE = "$volume OR $volume leaderboard"
const VOLUME_DETAILS = "Show your total trading volume across all linked wallets, or view the contest leaderboard"

type VolumeMultipleRequest struct {
	Users []string `json:"users"`
}

type VolumeMultipleResponse struct {
	Status string    `json:"status"`
	Data   []float32 `json:"data"`
}

type VolumeEntry struct {
	User   string  `json:"user"`
	Volume float32 `json:"volume"`
}

type VolumeBoardResponse struct {
	Status string        `json:"status"`
	Data   []VolumeEntry `json:"data"`
}

func VolumeCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Check if user wants leaderboard
	if len(args) > 0 && args[0] == "leaderboard" {
		showLeaderboard(database, s, m)
		return
	}

	// Regular volume command
	showUserVolume(database, s, m)
}

func showUserVolume(database db.Database, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ensure user exists
	database.EnsureUserExists(m.Author.ID)

	// Get user's linked wallets
	wallets, err := database.GetUserWallets(m.Author.ID)
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Error fetching your linked wallets")
		return
	}

	if len(wallets) == 0 {
		// No linked wallets
		embed := &discordgo.MessageEmbed{
			Title:       "Trading Volume",
			Color:       constants.IVY_GREEN,
			Description: "You have no linked wallets. Use `$link <wallet>` to link a wallet and start tracking your trading volume!",
		}
		s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return
	}

	// Fetch volume data from aggregator
	var volumes []float32
	var totalVolume float32

	if len(wallets) == 1 {
		// Use single wallet endpoint
		url := fmt.Sprintf("%s/volume/%s", constants.AGGREGATOR_URL, wallets[0])
		resp, err := http.Get(url)
		if err != nil {
			ReactErr(s, m)
			DmError(s, m.Author.ID, "Failed to fetch volume data")
			return
		}
		defer resp.Body.Close()

		var singleResponse struct {
			Status string  `json:"status"`
			Data   float32 `json:"data"`
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			ReactErr(s, m)
			DmError(s, m.Author.ID, "Failed to read volume data")
			return
		}

		err = json.Unmarshal(body, &singleResponse)
		if err != nil || singleResponse.Status != "ok" {
			ReactErr(s, m)
			DmError(s, m.Author.ID, "Failed to parse volume data")
			return
		}

		volumes = []float32{singleResponse.Data}
		totalVolume = singleResponse.Data
	} else {
		// Use multiple wallets endpoint
		reqBody := VolumeMultipleRequest{Users: wallets}
		jsonBody, err := json.Marshal(reqBody)
		if err != nil {
			ReactErr(s, m)
			DmError(s, m.Author.ID, "Failed to prepare request")
			return
		}

		url := fmt.Sprintf("%s/volume/multiple", constants.AGGREGATOR_URL)
		resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
		if err != nil {
			ReactErr(s, m)
			DmError(s, m.Author.ID, "Failed to fetch volume data")
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			ReactErr(s, m)
			DmError(s, m.Author.ID, "Failed to read volume data")
			return
		}

		var multiResponse VolumeMultipleResponse
		err = json.Unmarshal(body, &multiResponse)
		if err != nil || multiResponse.Status != "ok" {
			ReactErr(s, m)
			DmError(s, m.Author.ID, "Failed to parse volume data")
			return
		}

		volumes = multiResponse.Data
		for _, v := range volumes {
			totalVolume += v
		}
	}

	// Create response embed
	embed := &discordgo.MessageEmbed{
		Title: "📊 Trading Volume",
		Color: constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Total Volume",
				Value:  fmt.Sprintf("**$%.2f**", totalVolume),
				Inline: true,
			},
			{
				Name:   "Linked Wallets",
				Value:  fmt.Sprintf("**%d**", len(wallets)),
				Inline: true,
			},
		},
	}

	// Add breakdown if multiple wallets
	if len(wallets) > 1 {
		breakdown := ""
		for i, wallet := range wallets {
			// Truncate wallet address for display
			displayWallet := wallet
			if len(wallet) > 8 {
				displayWallet = fmt.Sprintf("%s...%s", wallet[:4], wallet[len(wallet)-4:])
			}
			breakdown += fmt.Sprintf("`%s`: $%.2f\n", displayWallet, volumes[i])
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Breakdown by Wallet",
			Value:  breakdown,
			Inline: false,
		})
	} else if len(wallets) == 1 {
		// Show the single wallet
		displayWallet := wallets[0]
		if len(wallets[0]) > 16 {
			displayWallet = fmt.Sprintf("%s...%s", wallets[0][:6], wallets[0][len(wallets[0])-6:])
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Wallet",
			Value:  fmt.Sprintf("`%s`", displayWallet),
			Inline: false,
		})
	}

	// Add footer with tip
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: "Volume is calculated from all-time trading activity on Ivy",
	}

	// Send the embed
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	ReactOk(s, m)
}

func showLeaderboard(database db.Database, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Get contest address
	contestAddress, err := database.GetContestAddress()
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Failed to retrieve contest address.")
		return
	}

	if contestAddress == "" {
		DmError(s, m.Author.ID, "No contest is currently active. Ask Violet to set one!")
		return
	}

	// Fetch leaderboard data from aggregator
	url := fmt.Sprintf("%s/games/%s/volume_board?count=25&skip=0", constants.AGGREGATOR_URL, contestAddress)
	resp, err := http.Get(url)
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Failed to fetch leaderboard data.")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Failed to read leaderboard data.")
		return
	}

	var volumeResponse VolumeBoardResponse
	err = json.Unmarshal(body, &volumeResponse)
	if err != nil || volumeResponse.Status != "ok" {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Failed to parse leaderboard data.")
		return
	}

	// Parse the response data into a more usable format
	entries := volumeResponse.Data
	wallets := make([]string, 0, len(entries))
	for _, entry := range entries {
		wallets = append(wallets, entry.User)
	}

	// Get wallet to user mapping
	walletToUser, err := database.GetWalletToUserMap(wallets)
	if err != nil {
		// Continue without user resolution if there's an error
		walletToUser = make(map[string]string)
	}

	// Build leaderboard embed
	embed := &discordgo.MessageEmbed{
		Title: "🏆 Volume Leaderboard",
		Color: constants.IVY_YELLOW,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Game: %s", contestAddress),
		},
	}

	if len(entries) == 0 {
		embed.Description = "No participants yet!"
	} else {
		var leaderboardText strings.Builder

		for i, entry := range entries {
			if i >= 25 { // Limit to top 25
				break
			}

			// Volume is already in USD from the aggregator
			totalVolumeUsd := entry.Volume

			// Format the display name (user mention or address)
			var displayName string
			if userID, exists := walletToUser[entry.User]; exists {
				displayName = fmt.Sprintf("<@%s>", userID)
			} else {
				// Truncate address for display
				if len(entry.User) > 8 {
					displayName = fmt.Sprintf("`%s...%s`", entry.User[:4], entry.User[len(entry.User)-4:])
				} else {
					displayName = fmt.Sprintf("`%s`", entry.User)
				}
			}

			// Add medal emoji for top 3
			var medal string
			switch i {
			case 0:
				medal = "🥇 "
			case 1:
				medal = "🥈 "
			case 2:
				medal = "🥉 "
			default:
				medal = fmt.Sprintf("**%d.** ", i+1)
			}

			// Format the line
			leaderboardText.WriteString(fmt.Sprintf(
				"%s%s - **$%.2f**\n",
				medal,
				displayName,
				totalVolumeUsd,
			))
		}

		embed.Description = leaderboardText.String()
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	ReactOk(s, m)
}
