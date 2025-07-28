// commands/rain.go
package commands

import (
	"database/sql"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func RainCommand(db *sql.DB, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Rain only works in guild channels
	if m.GuildID == "" {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Rain command can only be used in server channels, not DMs")
		return
	}

	if len(args) != 1 {
		util.ReactErr(s, m)
		util.DmUsage(s, m.Author.ID, "$rain amount", "Rain coins on active users in this server. Active users are those with an activity score of 5 or higher.")
		return
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
	amountRaw := uint64(amount * IVY_DECIMALS)

	// Ensure sender exists in database
	ensureUserExists(db, m.Author.ID)

	// Check sender's balance
	senderBalanceRaw, err := getUserBalanceRaw(db, m.Author.ID)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error checking balance")
		return
	}

	if senderBalanceRaw < amountRaw {
		senderBalance := float64(senderBalanceRaw) / IVY_DECIMALS
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("Insufficient balance. Your balance: **%.9f** IVY", senderBalance))
		return
	}

	// Get active users for this server
	activeUsers, err := getActiveUsersForRain(db, m.GuildID, constants.RAIN_ACTIVITY_REQUIREMENT)
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

	// Process the rain transaction
	amountPerUserRaw, err := processRain(db, m.Author.ID, eligibleUsers, amountRaw, senderBalanceRaw)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("Error processing rain: %v", err))
		return
	}

	// Get new balances for notifications
	newBalanceRaw, _ := getUserBalanceRaw(db, m.Author.ID)
	newBalance := float64(newBalanceRaw) / IVY_DECIMALS
	amountPerUser := float64(amountPerUserRaw) / IVY_DECIMALS

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
		recipientBalanceRaw, _ := getUserBalanceRaw(db, recipientID)
		recipientBalance := float64(recipientBalanceRaw) / IVY_DECIMALS
		util.DmSuccess(s, recipientID,
			fmt.Sprintf("You received **%.9f** IVY from <@%s>'s rain!\n\nYour new balance: **%.9f** IVY",
				amountPerUser, m.Author.ID, recipientBalance),
			"Rain Received",
			"")
	}
}
