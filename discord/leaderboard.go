package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

type VolumeEntry struct {
	User   string  `json:"user"`
	Volume float32 `json:"volume"`
}

type VolumeBoardResponse struct {
	Status string        `json:"status"`
	Data   []VolumeEntry `json:"data"`
}

const LEADERBOARD_USAGE = "$leaderboard"
const LEADERBOARD_DETAILS = "Show the contest leaderboard"

func LeaderboardCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
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
		Title: "🏆 Contest Leaderboard",
		Color: constants.IVY_YELLOW,
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Contest: %s", contestAddress),
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
