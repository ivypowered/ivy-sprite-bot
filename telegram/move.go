package telegram

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

func MoveCommand(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message, args []string) {
	// Check if it's a private chat
	if msg.Chat.Type != "private" {
		sendError(ctx, b, msg.Chat.ID, "Move commands must be used in private chat for security.")
		return
	}

	if len(args) != 2 {
		sendUsage(ctx, b, msg.Chat.ID, "/move", `Transfer funds from Telegram to Discord

<b>Usage:</b>
• /move [amount] [discord_id] - Move funds to Discord account

<b>Example:</b>
• /move 10.5 59502203051
• /move 0.1 123456789012345678

<b>Note:</b>
• The Discord account must already exist in Ivy Sprite
• Get your Discord ID by typing $id in Discord`)
		return
	}

	// Parse amount
	amount, err := strconv.ParseFloat(args[0], 64)
	if err != nil || amount <= 0 {
		sendError(ctx, b, msg.Chat.ID, "Please enter a valid positive amount")
		return
	}

	// Parse Discord ID
	discordID := args[1]
	_, err = strconv.ParseUint(discordID, 10, 64)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Please enter a valid Discord ID (numbers only)")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * db.IVY_DECIMALS)

	// Get user IDs
	telegramID := getDatabaseID(msg.From.ID)
	discordUserID := fmt.Sprintf("discord:%s", discordID)

	// Ensure Telegram user exists
	database.EnsureUserExists(telegramID)

	// Check if Discord user exists
	discordExists, err := database.IsUserExtant(discordUserID)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error checking Discord account")
		return
	}

	if !discordExists {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Discord user %s not found in Ivy Sprite. Make sure they have used the bot in Discord first.", discordID))
		return
	}

	// Check sender's balance
	senderBalanceRaw, err := database.GetUserBalanceRaw(telegramID)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error checking balance")
		return
	}

	if senderBalanceRaw < amountRaw {
		senderBalance := float64(senderBalanceRaw) / db.IVY_DECIMALS
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Insufficient balance. Your balance: <b>%.9f</b> IVY", senderBalance))
		return
	}

	// Perform transfer
	err = database.TransferFundsRaw(telegramID, discordUserID, amountRaw)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Error processing transfer: %v", err))
		return
	}

	// Get new balance
	newBalanceRaw, _ := database.GetUserBalanceRaw(telegramID)
	newBalance := float64(newBalanceRaw) / db.IVY_DECIMALS

	// Send success message
	sendSuccess(ctx, b, msg.Chat.ID,
		fmt.Sprintf("Successfully moved <b>%.9f IVY</b> to Discord user <code>%s</code>\n\nYour new balance: <b>%.9f IVY</b>\n\n💡 The recipient can check their balance in Discord with /balance",
			amount, discordID, newBalance),
		"✉️ Transfer to Discord Complete")
}
