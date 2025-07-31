package telegram

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func TipCommand(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message, args []string) {
	if len(args) < 1 || msg.ReplyToMessage == nil || msg.ReplyToMessage.From == nil {
		sendUsage(ctx, b, msg.Chat.ID, "/tip", `Send coins to another user

<b>Usage:</b>
• Reply to a message with /tip [amount]

<b>Examples:</b>
• /tip 10
• /tip $5

<b>Note:</b>
• Due to Telegram API limitations, it is not possible to support username mentions`)
		return
	}

	// Tip via reply
	recipientID := getDatabaseID(msg.ReplyToMessage.From.ID)
	recipientName := msg.ReplyToMessage.From.FirstName
	if msg.ReplyToMessage.From.Username != "" {
		recipientName = "@" + msg.ReplyToMessage.From.Username
	}

	// Get amount
	amount, err := util.ParseAmount(args[0])

	// Validate amount
	if err != nil || amount <= 0 {
		sendError(ctx, b, msg.Chat.ID, "Please enter a valid positive amount")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * db.IVY_DECIMALS)
	senderID := getDatabaseID(msg.From.ID)

	// Don't allow tipping yourself
	if senderID == recipientID {
		sendError(ctx, b, msg.Chat.ID, "You cannot tip yourself!")
		return
	}

	// Ensure both users exist in database
	database.EnsureUserExists(senderID)
	database.EnsureUserExists(recipientID)

	// Check sender's balance
	senderBalanceRaw, err := database.GetUserBalanceRaw(senderID)
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
	err = database.TransferFundsRaw(senderID, recipientID, amountRaw)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Error processing transfer: %v", err))
		return
	}

	// Get sender name for notifications
	senderName := msg.From.FirstName
	if msg.From.Username != "" {
		senderName = "@" + msg.From.Username
	}

	// Send brief public acknowledgment (reply to the tip message)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      fmt.Sprintf("🌿 %s tipped %.9f IVY to %s", escapeHTML(senderName), amount, escapeHTML(recipientName)),
		ParseMode: models.ParseModeHTML,
		ReplyParameters: &models.ReplyParameters{
			MessageID: msg.ID,
		},
	})
	if err != nil {
		// Fallback to non-reply if reply fails
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    msg.Chat.ID,
			Text:      fmt.Sprintf("🌿 %s tipped %.9f IVY to %s", escapeHTML(senderName), amount, escapeHTML(recipientName)),
			ParseMode: models.ParseModeHTML,
		})
	}

	// Send notification to recipient via DM
	recipientBalanceRaw, _ := database.GetUserBalanceRaw(recipientID)
	recipientBalance := float64(recipientBalanceRaw) / db.IVY_DECIMALS

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.ReplyToMessage.From.ID,
		Text: fmt.Sprintf(`<b>You received a tip!</b>

%s sent you <b>%.9f IVY</b>

🌿 Your new balance: <b>%.9f IVY</b>`,
			escapeHTML(senderName), amount, recipientBalance),
		ParseMode: models.ParseModeHTML,
	})
}
