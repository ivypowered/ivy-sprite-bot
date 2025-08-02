// telegram/rain.go
package telegram

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func RainCommand(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message, args []string) {
	// Handle check command (DM only)
	if len(args) == 1 && args[0] == "check" {
		if msg.Chat.Type != "private" {
			sendError(ctx, b, msg.Chat.ID, "The /rain check command can only be used in private messages")
			return
		}

		activeUsers, err := database.GetActiveUsersForRain("telegram", constants.RAIN_ACTIVITY_REQUIREMENT)
		if err != nil {
			sendError(ctx, b, msg.Chat.ID, "Error checking active users")
			return
		}

		// Remove the checking user from count if they're active
		senderID := getDatabaseID(msg.From.ID)
		eligibleCount := 0
		for _, userID := range activeUsers {
			if userID != senderID {
				eligibleCount++
			}
		}

		sendSuccess(ctx, b, msg.Chat.ID,
			fmt.Sprintf("Active users eligible for rain in the Ivy channel: <b>%d</b>\n\n<i>Users need an activity score of %d+ to receive rain.</i>",
				eligibleCount, constants.RAIN_ACTIVITY_REQUIREMENT),
			"🌧 Rain Status")
		return
	}

	// Rain only works in the main Ivy channel
	if msg.Chat.ID != IVY_TELEGRAM_CHANNEL_ID {
		sendError(ctx, b, msg.Chat.ID, "Rain command can only be used in the main Ivy channel")
		return
	}

	if len(args) < 1 {
		sendUsage(ctx, b, msg.Chat.ID, "/rain [amount]", `Rain coins on active users in the Ivy channel.

<b>Usage:</b>
• /rain [amount] - Rain on active users
• /rain [amount] max=[number] - Rain on up to [number] active users
• /rain check - Check eligible users (DM only)

<b>Examples:</b>
• /rain 10
• /rain 5.5 max=20`)
		return
	}

	// Parse max users parameter
	var maxUsers int = math.MaxInt
	if len(args) >= 2 {
		maxp := args[1]
		if strings.HasPrefix(maxp, "max=") {
			var err error
			maxUsers, err = strconv.Atoi(maxp[4:])
			if err != nil || maxUsers <= 0 {
				sendError(ctx, b, msg.Chat.ID, "Invalid max users parameter")
				return
			}
		}
	}

	// Parse amount
	amount, err := util.ParseAmount(args[0])
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Please enter a valid positive amount")
		return
	}

	// Enforce minimum
	price := constants.PRICE.Get(constants.RPC_CLIENT)
	rainMin := (math.Max(0, (constants.RAIN_MIN_AMOUNT_USD-0.01)) / price) // $0.01 threshold
	if amount < rainMin {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf(
			"Rain amount must be at least $%.2f (%.9f IVY)", constants.RAIN_MIN_AMOUNT_USD, rainMin,
		))
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * constants.IVY_FACTOR)

	// Ensure sender exists in database
	senderID := getDatabaseID(msg.From.ID)
	database.EnsureUserExists(senderID)

	// Check sender's balance
	senderBalanceRaw, err := database.GetUserBalanceRaw(senderID)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error checking balance")
		return
	}

	if senderBalanceRaw < amountRaw {
		senderBalance := float64(senderBalanceRaw) / constants.IVY_FACTOR
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Insufficient balance. Your balance: <b>%.9f</b> IVY", senderBalance))
		return
	}

	// Get active users for the Ivy channel (using "telegram" as the server identifier)
	activeUsers, err := database.GetActiveUsersForRain("telegram", constants.RAIN_ACTIVITY_REQUIREMENT)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error finding active users")
		return
	}

	// Remove sender from active users list (can't rain on yourself)
	var eligibleUsers []string
	for _, userID := range activeUsers {
		if userID != senderID {
			eligibleUsers = append(eligibleUsers, userID)
		}
	}

	if len(eligibleUsers) == 0 {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("No active users found. Users need an activity score of %d+ in this channel to receive rain.", constants.RAIN_ACTIVITY_REQUIREMENT))
		return
	}

	// Bound by maximum users
	if len(eligibleUsers) > maxUsers {
		eligibleUsers = eligibleUsers[:maxUsers]
	}

	// Process the rain transaction
	amountPerUserRaw, err := database.ProcessRain(senderID, eligibleUsers, amountRaw, senderBalanceRaw)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Error processing rain: %v", err))
		return
	}

	// Get new balances for notifications
	newBalanceRaw, _ := database.GetUserBalanceRaw(senderID)
	newBalance := float64(newBalanceRaw) / constants.IVY_FACTOR
	amountPerUser := float64(amountPerUserRaw) / constants.IVY_FACTOR

	// Get sender name
	senderName := msg.From.FirstName
	if msg.From.Username != "" {
		senderName = "@" + msg.From.Username
	}

	// Send confirmation in channel
	confirmText := fmt.Sprintf(`💧 <b>Rain Complete!</b>

%s rained <b>%.9f IVY</b> on <b>%d</b> active users!

Each user received <b>%.9f IVY</b>

<i>Stay active to receive future rains!</i>`,
		escapeHTML(senderName), amount, len(eligibleUsers), amountPerUser)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      confirmText,
		ParseMode: models.ParseModeHTML,
	})

	// DM sender confirmation
	senderDMText := fmt.Sprintf(`Successfully rained <b>%.9f IVY</b> on <b>%d</b> active users

<b>Amount per user:</b> %.9f IVY
<b>Your new balance:</b> %.9f IVY`,
		amount, len(eligibleUsers), amountPerUser, newBalance)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.From.ID,
		Text:      "💧 <b>Rain Sent</b>\n\n" + senderDMText,
		ParseMode: models.ParseModeHTML,
	})

	// DM each recipient
	for _, recipientID := range eligibleUsers {
		tgID, err := fromDatabaseID(recipientID)
		if err != nil {
			// should not happen
			log.Printf("can't extract tg id from db id: %v", err)
			continue
		}

		recipientBalanceRaw, _ := database.GetUserBalanceRaw(recipientID)
		recipientBalance := float64(recipientBalanceRaw) / constants.IVY_FACTOR

		recipientText := fmt.Sprintf(`You received <b>%.9f IVY</b> from %s's rain!

<b>Your new balance:</b> %.9f IVY`,
			amountPerUser, escapeHTML(senderName), recipientBalance)

		// Try to send DM to recipient
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    tgID,
			Text:      "💧 <b>Rain Received</b>\n\n" + recipientText,
			ParseMode: models.ParseModeHTML,
		})
	}
}
