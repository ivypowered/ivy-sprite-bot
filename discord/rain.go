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

const RAIN_USAGE_NAME string = "$rain amount OR $rain channels [add|remove|list|clear]"
const RAIN_USAGE_DETAILS string = `Rain coins on active users in whitelisted channels.

Channel Management:
• $rain channels add #channel1 #channel2 - Add channels to whitelist
• $rain channels remove #channel1 - Remove channels from whitelist
• $rain channels list - Show whitelisted channels
• $rain channels clear - Clear all whitelisted channels

Rain Usage:
• $rain amount - Rain on active users (requires whitelisted channels)
• $rain amount max=[amount] - Rain on up to [amount] active users`

func RainCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Handle check command
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

	// Handle channel management subcommands
	if args[0] == "channels" {
		handleRainChannels(database, args[1:], s, m)
		return
	}

	// Check if any channels are whitelisted
	rainChannels, err := database.GetRainChannels(m.GuildID)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error checking rain channels")
		return
	}

	if len(rainChannels) == 0 {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "No channels are whitelisted for rain. Use `$rain channels add #channel` to add channels.")
		return
	}

	// Parse max users parameter
	var maxUsers int = math.MaxInt
	if len(args) >= 2 {
		maxp := args[1]
		if strings.HasPrefix(maxp, "max=") {
			var err error
			maxUsers, err = strconv.Atoi(maxp[4:])
			if err != nil || maxUsers < 0 {
				util.ReactErr(s, m)
				util.DmUsage(s, m.Author.ID, RAIN_USAGE_NAME, RAIN_USAGE_DETAILS)
				return
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
		util.DmError(s, m.Author.ID, fmt.Sprintf("No active users found in whitelisted channels. Users need an activity score of %d+ in whitelisted channels to receive rain.", constants.RAIN_ACTIVITY_REQUIREMENT))
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

func handleRainChannels(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	if len(args) == 0 {
		util.ReactErr(s, m)
		util.DmUsage(s, m.Author.ID, RAIN_USAGE_NAME, RAIN_USAGE_DETAILS)
		return
	}

	switch args[0] {
	case "add":
		if len(args) != 2 {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "Please mention exactly one channel to add")
			return
		}
		// Check if user is violet
		if m.Author.ID != "1348921951493554277" {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "For now only violet can change these variables")
			return
		}

		// Parse channel mention
		channelID := strings.TrimPrefix(strings.TrimSuffix(args[1], ">"), "<#")

		// Verify channel exists in this guild
		channel, err := s.Channel(channelID)
		if err != nil || channel.GuildID != m.GuildID {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "Invalid channel or channel not found in this server")
			return
		}

		if channel.Type != discordgo.ChannelTypeGuildText {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "Only text channels can be added to rain whitelist")
			return
		}

		err = database.AddRainChannel(m.GuildID, channelID)
		if err != nil {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "Failed to add channel to rain whitelist")
			return
		}

		util.ReactOk(s, m)
		util.DmSuccess(s, m.Author.ID, fmt.Sprintf("Added <#%s> to rain whitelist", channelID), "Channel Added", "")

	case "remove":
		if len(args) != 2 {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "Please mention exactly one channel to remove")
			return
		}
		// Check if user is violet
		if m.Author.ID != "1348921951493554277" {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "For now only violet can change these variables")
			return
		}

		// Parse channel mention
		channelID := strings.TrimPrefix(strings.TrimSuffix(args[1], ">"), "<#")

		err := database.RemoveRainChannel(m.GuildID, channelID)
		if err != nil {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "Failed to remove channel from rain whitelist")
			return
		}

		util.ReactOk(s, m)
		util.DmSuccess(s, m.Author.ID, fmt.Sprintf("Removed <#%s> from rain whitelist", channelID), "Channel Removed", "")

	case "list":
		channels, err := database.GetRainChannels(m.GuildID)
		if err != nil {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "Error retrieving channel list")
			return
		}

		if len(channels) == 0 {
			util.ReactOk(s, m)
			util.DmSuccess(s, m.Author.ID, "No channels are currently whitelisted for rain", "Rain Channels", "")
			return
		}

		channelList := ""
		for _, channelID := range channels {
			channelList += fmt.Sprintf("• <#%s>\n", channelID)
		}

		util.ReactOk(s, m)
		util.DmSuccess(s, m.Author.ID, channelList, "Rain Channels", fmt.Sprintf("Total: %d channels", len(channels)))

	case "clear":
		// Check if user is violet
		if m.Author.ID != "1348921951493554277" {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "For now only violet can change these variables")
			return
		}
		err := database.ClearRainChannels(m.GuildID)
		if err != nil {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, "Error clearing channel list")
			return
		}

		util.ReactOk(s, m)
		util.DmSuccess(s, m.Author.ID, "All channels have been removed from the rain whitelist", "Channels Cleared", "")

	default:
		util.ReactErr(s, m)
		util.DmUsage(s, m.Author.ID, RAIN_USAGE_NAME, RAIN_USAGE_DETAILS)
	}
}
