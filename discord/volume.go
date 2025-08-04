package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

const VOLUME_USAGE = "$volume"
const VOLUME_DETAILS = "Show your total trading volume across all linked wallets"

type VolumeMultipleRequest struct {
	Users []string `json:"users"`
}

type VolumeMultipleResponse struct {
	Status string    `json:"status"`
	Data   []float32 `json:"data"`
}

func VolumeCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
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
