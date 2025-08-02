package telegram

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func getWithdrawUrl(withdrawId, userId, userName, signature string) string {
	return fmt.Sprintf(
		"https://sprite.ivypowered.com/withdraw?withdraw_id=%s&user_id=%s&name=%s&authority=%s&signature=%s",
		withdrawId,
		userId,
		url.QueryEscape(userName),
		constants.WITHDRAW_AUTHORITY_B58,
		signature,
	)
}

func WithdrawCommand(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message, args []string) {
	// Check if it's a private chat
	if msg.Chat.Type != "private" {
		sendError(ctx, b, msg.Chat.ID, "Withdrawals can only be processed in private chat for security. Please send this command directly to me.")
		return
	}

	if len(args) == 0 || (args[0] != "list" && len(args) != 2) {
		sendUsage(ctx, b, msg.Chat.ID, "/withdraw", `Withdraw coins from your account

<b>Usage:</b>
• /withdraw [amount] [sol_address] - Create withdrawal
• /withdraw list - List recent withdrawals

<b>Example:</b>
• /withdraw 0.5 A32dqo7aTp3eHhxpSA6Cw67zWosKc3ymiYz2DbPVx8BK`)
		return
	}

	if args[0] == "list" {
		listWithdrawals(ctx, database, b, msg)
		return
	}

	amount, err := strconv.ParseFloat(args[0], 64)
	if err != nil || amount <= 0 {
		sendError(ctx, b, msg.Chat.ID, "Please enter a valid positive amount")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * constants.IVY_FACTOR)

	userKey, err := solana.PublicKeyFromBase58(args[1])
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Please enter a valid base58-encoded Solana address")
		return
	}

	userID := getDatabaseID(msg.From.ID)
	database.EnsureUserExists(userID)

	balanceRaw, err := database.GetUserBalanceRaw(userID)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error checking balance")
		return
	}

	if balanceRaw < amountRaw {
		balance := float64(balanceRaw) / constants.IVY_FACTOR
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Insufficient balance. Your balance: <b>%.9f</b> IVY", balance))
		return
	}

	// Generate withdrawal ID
	withdrawIDBytes := util.GenerateID(amountRaw)
	withdrawID := hex.EncodeToString(withdrawIDBytes[:])

	// Sign withdrawal
	signature := util.SignWithdrawal(constants.SPRITE_VAULT, userKey, withdrawIDBytes, constants.WITHDRAW_AUTHORITY_KEY)
	signatureHex := hex.EncodeToString(signature[:])

	// Create withdrawal and debit user atomically
	err = database.CreateWithdrawal(withdrawID, userID, balanceRaw, amountRaw, signatureHex)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Error processing withdrawal: %v", err))
		return
	}

	// Get new balance
	newBalanceRaw, _ := database.GetUserBalanceRaw(userID)
	newBalance := float64(newBalanceRaw) / constants.IVY_FACTOR

	// Get user info
	username := msg.From.Username
	if username == "" {
		username = msg.From.FirstName
	}

	// Create withdrawal URL
	withdrawURL := getWithdrawUrl(
		withdrawID,
		userID,
		username,
		signatureHex,
	)

	// Send success message
	text := fmt.Sprintf(`⤴ <b>Withdrawal Created</b>

🌿 <b>Amount:</b> %.9f IVY
💳 <b>New Balance:</b> %.9f IVY
🔖 <b>Withdrawal ID:</b> <code>%s</code>

🔗 <b>Claim Link:</b>
%s

⚡ Click the link above to claim your withdrawal`,
		amount,
		newBalance,
		withdrawID[:8]+"...",
		withdrawURL)

	isDisabled := true
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
	})
}

func listWithdrawals(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message) {
	userID := getDatabaseID(msg.From.ID)
	withdrawals, err := database.ListWithdrawals(userID, 10)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error fetching withdrawals")
		return
	}

	if len(withdrawals) == 0 {
		sendInfo(ctx, b, msg.Chat.ID, "📤 Recent Withdrawals", "No withdrawals found")
		return
	}

	var text strings.Builder
	text.WriteString("📤 <b>Recent Withdrawals</b>\n\n")

	username := msg.From.Username
	if username == "" {
		username = msg.From.FirstName
	}

	for i, withdrawal := range withdrawals {
		amount := float64(withdrawal.AmountRaw) / constants.IVY_FACTOR

		withdrawURL := getWithdrawUrl(
			withdrawal.WithdrawID,
			userID,
			username,
			withdrawal.Signature,
		)

		text.WriteString(fmt.Sprintf("%d. <b>%.9f IVY</b>\n", i+1, amount))
		text.WriteString(fmt.Sprintf("   ID: <code>%s</code>\n", withdrawal.WithdrawID[:8]+"..."))
		text.WriteString(fmt.Sprintf("   <a href=\"%s\">Claim Link</a>\n\n", withdrawURL))
	}

	isDisabled := true
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      text.String(),
		ParseMode: models.ParseModeHTML,
		LinkPreviewOptions: &models.LinkPreviewOptions{
			IsDisabled: &isDisabled,
		},
	})
}
