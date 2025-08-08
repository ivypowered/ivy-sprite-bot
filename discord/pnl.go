// discord/pnl.go
package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/gagliardetto/solana-go"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

const PNL_USAGE = "$pnl OR $pnl <address> OR $pnl leaderboard [realized]"
const PNL_DETAILS = `View your profit and loss statistics.

Commands:
• $pnl - Show PnL for the current contest
• $pnl <address> - Show PnL for a specific game
• $pnl leaderboard - Show the PnL leaderboard for current contest
• $pnl leaderboard realized - Show only realized gains leaderboard`

type PnlResponse struct {
	InUsd    float32 `json:"in_usd"`
	OutUsd   float32 `json:"out_usd"`
	Position float32 `json:"position"`
	Price    float32 `json:"price"`
}

type PnlApiResponse struct {
	Status string      `json:"status"`
	Data   PnlResponse `json:"data"`
}

type PnlEntry struct {
	User     string  `json:"user"`
	InUsd    float32 `json:"in_usd"`
	OutUsd   float32 `json:"out_usd"`
	Position float32 `json:"position"`
}

type PnlLeaderboardResponse struct {
	Status string     `json:"status"`
	Data   []PnlEntry `json:"data"`
}

func PnlCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ensure user exists
	database.EnsureUserExists(m.Author.ID)

	// Handle subcommands
	if len(args) == 0 {
		// Show PnL for current contest
		showContestPnl(database, s, m)
		return
	}

	if args[0] == "leaderboard" {
		// Check for "realized" modifier
		realized := len(args) > 1 && args[1] == "realized"
		showPnlLeaderboard(database, realized, s, m)
		return
	}

	// Otherwise, treat as game address
	showGamePnl(database, args[0], s, m)
}

func showContestPnl(database db.Database, s *discordgo.Session, m *discordgo.MessageCreate) {
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

	showGamePnl(database, contestAddress, s, m)
}

func showGamePnl(database db.Database, gameAddress string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Validate game address
	_, err := solana.PublicKeyFromBase58(gameAddress)
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Invalid game address format")
		return
	}

	// Get user's linked wallets
	wallets, err := database.GetUserWallets(m.Author.ID)
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Error fetching your linked wallets")
		return
	}

	if len(wallets) == 0 {
		DmError(s, m.Author.ID, "You have no linked wallets. Use `$link <wallet>` to link a wallet.")
		return
	}

	// Aggregate PnL data across all linked wallets
	var totalInUsd, totalOutUsd, totalPositionValue float32
	var currentPrice float32
	hasData := false

	for _, wallet := range wallets {
		// Fetch PnL data from aggregator
		url := fmt.Sprintf("%s/games/%s/pnl/%s", constants.AGGREGATOR_URL, gameAddress, wallet)
		resp, err := http.Get(url)
		if err != nil {
			continue // Skip this wallet on error
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		var pnlResp PnlApiResponse
		err = json.Unmarshal(body, &pnlResp)
		if err != nil || pnlResp.Status != "ok" {
			continue
		}

		// Aggregate data
		totalInUsd += pnlResp.Data.InUsd
		totalOutUsd += pnlResp.Data.OutUsd
		totalPositionValue += pnlResp.Data.Position * pnlResp.Data.Price
		currentPrice = pnlResp.Data.Price // Will be the same for all wallets
		hasData = true
	}

	if !hasData {
		DmError(s, m.Author.ID, "No trading data found for this game.")
		return
	}

	// Calculate PnL metrics
	pnlMetrics := calculatePnlMetrics(totalInUsd, totalOutUsd, totalPositionValue)

	// Create embed
	embed := &discordgo.MessageEmbed{
		Title: "📊 Profit & Loss",
		Color: getPnlColor(pnlMetrics.pnlPercent),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Total In",
				Value:  fmt.Sprintf("$%.2f", totalInUsd),
				Inline: true,
			},
			{
				Name:   "Total Out",
				Value:  fmt.Sprintf("$%.2f", totalOutUsd),
				Inline: true,
			},
			{
				Name:   "Position Value",
				Value:  fmt.Sprintf("$%.2f", totalPositionValue),
				Inline: true,
			},
			{
				Name:   "PnL",
				Value:  formatPnlPercent(pnlMetrics.pnlPercent),
				Inline: true,
			},
			{
				Name:   "Realized",
				Value:  fmt.Sprintf("%.1f%%", 100.0-pnlMetrics.unrealizedPercent),
				Inline: true,
			},
			{
				Name:   "Current Price",
				Value:  fmt.Sprintf("$%.4f", currentPrice),
				Inline: true,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Game: %s", gameAddress),
		},
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	ReactOk(s, m)
}

func showPnlLeaderboard(database db.Database, realized bool, s *discordgo.Session, m *discordgo.MessageCreate) {
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
	url := fmt.Sprintf("%s/games/%s/pnl_board?count=25&skip=0&realized=%t", constants.AGGREGATOR_URL, contestAddress, realized)
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

	var pnlResponse PnlLeaderboardResponse
	err = json.Unmarshal(body, &pnlResponse)
	if err != nil || pnlResponse.Status != "ok" {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Failed to parse leaderboard data.")
		return
	}

	// Get current price for the game
	var currentPrice float32
	if len(pnlResponse.Data) > 0 {
		// Make a quick call to get the price (using first wallet in leaderboard)
		priceUrl := fmt.Sprintf("%s/games/%s/pnl/%s", constants.AGGREGATOR_URL, contestAddress, pnlResponse.Data[0].User)
		if priceResp, err := http.Get(priceUrl); err == nil {
			defer priceResp.Body.Close()
			if priceBody, err := io.ReadAll(priceResp.Body); err == nil {
				var priceData PnlApiResponse
				if json.Unmarshal(priceBody, &priceData) == nil && priceData.Status == "ok" {
					currentPrice = priceData.Data.Price
				}
			}
		}
	}

	// Get wallet to user mapping
	wallets := make([]string, 0, len(pnlResponse.Data))
	for _, entry := range pnlResponse.Data {
		wallets = append(wallets, entry.User)
	}
	walletToUser, err := database.GetWalletToUserMap(wallets)
	if err != nil {
		walletToUser = make(map[string]string)
	}

	// Build leaderboard embed
	title := "🌿 Profit-and-Loss Leaderboard"
	if realized {
		title = "🌿 Realized Profit-and-Loss Leaderboard"
	}

	embed := &discordgo.MessageEmbed{
		Title: title,
		Color: constants.IVY_GREEN,
	}

	if len(pnlResponse.Data) == 0 {
		embed.Description = "No trading activity yet!"
	} else {
		var leaderboardText strings.Builder

		// Add current price for context (only for unrealized leaderboard)
		if !realized && currentPrice > 0 {
			leaderboardText.WriteString(fmt.Sprintf("**Current Price:** $%.4f\n\n", currentPrice))
		}

		for i, entry := range pnlResponse.Data {
			if i >= 15 {
				break
			}

			// Format rank with medals
			var rankEmoji string
			switch i {
			case 0:
				rankEmoji = "🥇"
			case 1:
				rankEmoji = "🥈"
			case 2:
				rankEmoji = "🥉"
			default:
				rankEmoji = fmt.Sprintf("**#%d**", i+1)
			}

			// Get player display name
			displayName := getPlayerDisplayName(entry.User, walletToUser)

			if realized {
				// For realized leaderboard, only show realized gains
				realizedPnl := calculateRealizedPnl(entry.InUsd, entry.OutUsd)

				// Status indicator for profit/loss
				var statusEmoji string
				if realizedPnl > 0 {
					statusEmoji = "📈"
				} else if realizedPnl < 0 {
					statusEmoji = "📉"
				} else {
					statusEmoji = "➖"
				}

				// Format the entry for realized leaderboard
				leaderboardText.WriteString(fmt.Sprintf(
					"%s %s\n**Realized PnL:** %+.1f%% %s\n\n",
					rankEmoji,
					displayName,
					realizedPnl,
					statusEmoji,
				))
			} else {
				// For unrealized leaderboard, show full metrics
				positionValue := entry.Position * currentPrice
				metrics := calculatePnlMetrics(entry.InUsd, entry.OutUsd, positionValue)
				realizedPercent := 100 - metrics.unrealizedPercent

				// Status indicator
				var statusEmoji string
				if realizedPercent >= 100 {
					statusEmoji = "✅"
				} else {
					statusEmoji = ""
				}

				realizedDisplay := "Fully realized"
				if realizedPercent < 99 {
					realizedDisplay = fmt.Sprintf("%.2f%%", realizedPercent)
				}

				// Format the entry for unrealized leaderboard
				leaderboardText.WriteString(fmt.Sprintf(
					"%s %s\n**PnL:** %+.1f%% • **Realized:** %s %s\n\n",
					rankEmoji,
					displayName,
					metrics.pnlPercent,
					realizedDisplay,
					statusEmoji,
				))
			}
		}

		// Add legend
		leaderboardText.WriteString("━━━━━━━━━━━━━━━━━━━━━\n")

		embed.Description = leaderboardText.String()
	}

	// Different footer messages based on leaderboard type
	if realized {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: "🏆 Only realized gains count for prizes!",
		}
	} else {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: "💡 Tip: Unrealized gains don't count for prizes!",
		}
	}

	s.ChannelMessageSendEmbed(m.ChannelID, embed)
	ReactOk(s, m)
}

// Helper function for clean display names
func getPlayerDisplayName(wallet string, walletToUser map[string]string) string {
	if userID, exists := walletToUser[wallet]; exists {
		// Return Discord mention format - this will work properly outside code blocks
		return fmt.Sprintf("<@%s>", userID)
	}

	// Shorten wallet addresses and use inline code formatting for clarity
	if len(wallet) > 8 {
		return fmt.Sprintf("`%s...%s`", wallet[:4], wallet[len(wallet)-4:])
	}
	return fmt.Sprintf("`%s`", wallet)
}

// Helper types and functions
type pnlMetrics struct {
	pnlPercent        float32
	unrealizedPercent float32
}

func calculatePnlMetrics(inUsd, outUsd, positionValue float32) pnlMetrics {
	totalOut := outUsd + positionValue

	// Calculate PnL percentage
	var pnlPercent float32
	if inUsd > 0 {
		pnlPercent = ((totalOut - inUsd) / inUsd) * 100
	}

	// Calculate unrealized percentage
	var unrealizedPercent float32
	if totalOut > 0 {
		unrealizedPercent = (positionValue / totalOut) * 100
	}

	return pnlMetrics{
		pnlPercent:        pnlPercent,
		unrealizedPercent: unrealizedPercent,
	}
}

func calculateRealizedPnl(inUsd, outUsd float32) float32 {
	if inUsd > 0 {
		return ((outUsd - inUsd) / inUsd) * 100
	}
	return 0
}

func formatPnlPercent(percent float32) string {
	if percent >= 0 {
		return fmt.Sprintf("**+%.2f%%**", percent)
	}
	return fmt.Sprintf("**%.2f%%**", percent)
}

func getPnlColor(pnlPercent float32) int {
	if pnlPercent > 0 {
		return constants.IVY_GREEN
	} else if pnlPercent < 0 {
		return constants.IVY_RED
	}
	return constants.IVY_WHITE
}
