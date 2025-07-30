// discord/rain.go
package discord

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

const RAIN_USAGE_NAME string = "$rain amount OR $rain amount max=10"
const RAIN_USAGE_DETAILS string = "Rain coins on active users in this server. Active users are those with an activity score of 5 or higher."

func RainCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Check how many people would receive the rain
	if len(args) == 2 && args[0] == "check" {
		server := args[1]
		activeUsers, err := database.GetActiveUsersForRain(server, constants.RAIN_ACTIVITY_REQUIREMENT)
		if err != nil {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, err.Error())
			return
		}
		util.ReactOk(s, m)
		util.DmSuccess(s, m.Author.ID, fmt.Sprintf("Active users for %s: **%d**", server, len(activeUsers)), "Rain information", "")
		return
	}

	// Rain only works in guild channels
	if m.GuildID == "" {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Rain command can only be used in server channels, not DMs")
		return
	}

	if len(args) < 1 {
		util.ReactErr(s, m)
		util.DmUsage(s, m.Author.ID, RAIN_USAGE_NAME, RAIN_USAGE_DETAILS)
		return
	}

	var maxUsers int = math.MaxInt
	if len(args) >= 2 {
		maxp := args[1]
		if strings.HasPrefix(maxp, "max=") {
			var err error
			maxUsers, err = strconv.Atoi(maxp[4:])
			if err != nil || maxUsers < 0 {
				util.ReactErr(s, m)
				util.DmUsage(s, m.Author.ID, RAIN_USAGE_NAME, RAIN_USAGE_DETAILS)
			}
		}
	}

	// Parse amount
	amount, err := util.ParseAmount(args[0])
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Please enter a valid positive amount")
		return
	}

	// Enforce minimum
	if amount < constants.RAIN_MIN_AMOUNT {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("Rain amount must be at least %.9f IVY", constants.RAIN_MIN_AMOUNT))
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * db.IVY_DECIMALS)

	// Ensure sender exists in database
	database.EnsureUserExists(m.Author.ID)

	// Check sender's balance
	senderBalanceRaw, err := database.GetUserBalanceRaw(m.Author.ID)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error checking balance")
		return
	}

	if senderBalanceRaw < amountRaw {
		senderBalance := float64(senderBalanceRaw) / db.IVY_DECIMALS
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("Insufficient balance. Your balance: **%.9f** IVY", senderBalance))
		return
	}

	// Get active users for this server
	activeUsers, err := database.GetActiveUsersForRain(m.GuildID, constants.RAIN_ACTIVITY_REQUIREMENT)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error finding active users")
		return
	}

	// Remove sender from active users list (can't rain on yourself)
	var eligibleUsers []string
	for _, userID := range activeUsers {
		if userID != m.Author.ID {
			eligibleUsers = append(eligibleUsers, userID)
		}
	}

	if len(eligibleUsers) == 0 {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("No active users found in this server. Users need an activity score of %d+ to receive rain.", constants.RAIN_ACTIVITY_REQUIREMENT))
		return
	}

	// Bound by maximum users
	if len(eligibleUsers) > maxUsers {
		eligibleUsers = eligibleUsers[:maxUsers]
	}

	// Process the rain transaction
	amountPerUserRaw, err := database.ProcessRain(m.Author.ID, eligibleUsers, amountRaw, senderBalanceRaw)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("Error processing rain: %v", err))
		return
	}

	// Get new balances for notifications
	newBalanceRaw, _ := database.GetUserBalanceRaw(m.Author.ID)
	newBalance := float64(newBalanceRaw) / db.IVY_DECIMALS
	amountPerUser := float64(amountPerUserRaw) / db.IVY_DECIMALS

	util.ReactOk(s, m)

	// Send confirmation in channel
	_, _ = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Title:       "💧 Rain Complete!",
		Description: fmt.Sprintf("<@%s> rained **%.9f** IVY on **%d** active users!\n\nEach user received **%.9f** IVY", m.Author.ID, amount, len(eligibleUsers), amountPerUser),
		Color:       0x00ff00,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Stay active to receive future rains!",
		},
	})

	// DM sender confirmation
	util.DmSuccess(s, m.Author.ID,
		fmt.Sprintf("Successfully rained **%.9f** IVY on **%d** active users\n\nAmount per user: **%.9f** IVY\nYour new balance: **%.9f** IVY",
			amount, len(eligibleUsers), amountPerUser, newBalance),
		"Rain Sent",
		"")

	// DM each recipient
	for _, recipientID := range eligibleUsers {
		recipientBalanceRaw, _ := database.GetUserBalanceRaw(recipientID)
		recipientBalance := float64(recipientBalanceRaw) / db.IVY_DECIMALS
		util.DmSuccess(s, recipientID,
			fmt.Sprintf("You received **%.9f** IVY from <@%s>'s rain!\n\nYour new balance: **%.9f** IVY",
				amountPerUser, m.Author.ID, recipientBalance),
			"Rain Received",
			"")
	}
}
