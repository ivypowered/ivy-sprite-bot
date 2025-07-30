package telegram

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func DepositCommand(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message, args []string) {
	// Check if it's a private chat
	if msg.Chat.Type != "private" {
		sendError(ctx, b, msg.Chat.ID, "Deposit commands must be used in private chat for security.")
		return
	}

	if len(args) == 0 {
		sendUsage(ctx, b, msg.Chat.ID, "/deposit", `Create a new deposit or check an existing one

<b>Usage:</b>
• /deposit [amount] - Create new deposit
• /deposit check [id] - Check deposit status
• /deposit list - List recent deposits

<b>Examples:</b>
• /deposit 0.75
• /deposit check 3a8fb7`)
		return
	}

	// Check if this is a deposit check
	if args[0] == "check" {
		if len(args) != 2 {
			sendUsage(ctx, b, msg.Chat.ID, "/deposit check", "Check the status of a pending deposit\n\n<b>Example:</b> /deposit check 3a8fb7")
			return
		}
		checkDeposit(ctx, database, b, msg, args[1])
		return
	}

	if args[0] == "list" {
		listDeposits(ctx, database, b, msg)
		return
	}

	// Parse amount for new deposit
	amount, err := strconv.ParseFloat(args[0], 64)
	if err != nil || amount <= 0 {
		sendError(ctx, b, msg.Chat.ID, "Please enter a valid positive amount")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * db.IVY_DECIMALS)
	userID := getDatabaseID(msg.From.ID)

	database.EnsureUserExists(userID)

	// Generate deposit ID
	depositIDBytes := util.GenerateID(amountRaw)
	depositID := hex.EncodeToString(depositIDBytes[:])

	// Create deposit record
	err = database.CreateDeposit(depositID, userID, amountRaw)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error creating deposit")
		return
	}

	// Create deposit URL
	username := msg.From.Username
	if username == "" {
		username = msg.From.FirstName
	}

	depositURL := fmt.Sprintf(
		"https://sprite.ivypowered.com/deposit?deposit_id=%s&user_id=%s&name=%s",
		depositID,
		userID,
		url.QueryEscape(username),
	)

	// Send success message
	text := fmt.Sprintf(`✅ <b>Deposit Created</b>

🌿 <b>Amount:</b> %.9f IVY
🔖 <b>Deposit ID:</b> <code>%s</code>

📋 <b>Instructions:</b>
1. Click the link below to complete your deposit
2. After sending, use /deposit check %s to verify

🔗 <b>Deposit Link:</b>
%s`,
		amount,
		depositID[:8]+"...",
		depositID[:6],
		depositURL)

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

func checkDeposit(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message, depositIDPrefix string) {
	userID := getDatabaseID(msg.From.ID)

	// Find matching deposit
	fullDepositID, amountRaw, completed, err := database.FindDepositByPrefix(userID, depositIDPrefix)

	if err == sql.ErrNoRows {
		sendError(ctx, b, msg.Chat.ID, "No deposit found with that ID")
		return
	} else if err != nil {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Error checking deposit: %v", err))
		return
	}

	if completed == 1 {
		sendSuccess(ctx, b, msg.Chat.ID, "This deposit has already been completed!", "✅ Deposit Already Processed")
		return
	}

	// Decode deposit ID
	depositIDBytes, err := hex.DecodeString(fullDepositID)
	if err != nil || len(depositIDBytes) != 32 {
		sendError(ctx, b, msg.Chat.ID, "Invalid deposit ID format")
		return
	}
	var depositID32 [32]byte
	copy(depositID32[:], depositIDBytes)

	// Check if deposit is complete on-chain
	isComplete, err := util.IsDepositComplete(constants.RPC_CLIENT, constants.SPRITE_VAULT, depositID32)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, fmt.Sprintf("Error checking deposit status: %v", err))
		return
	}

	if !isComplete {
		sendClock(ctx, b, msg.Chat.ID, "Deposit Incomplete", fmt.Sprintf("Deposit <code>%s...</code> is not yet complete. Please try again later!", fullDepositID[:8]))
		return
	}

	// Complete the deposit
	err = database.CompleteDeposit(fullDepositID)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error completing deposit")
		return
	}

	// Get new balance
	newBalanceRaw, _ := database.GetUserBalanceRaw(userID)
	newBalance := float64(newBalanceRaw) / db.IVY_DECIMALS
	amount := float64(amountRaw) / db.IVY_DECIMALS

	sendSuccess(ctx, b, msg.Chat.ID,
		fmt.Sprintf("Deposited <b>%.9f IVY</b>\nNew balance: <b>%.9f IVY</b>", amount, newBalance),
		"✅ Deposit Complete")
}

func listDeposits(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message) {
	userID := getDatabaseID(msg.From.ID)
	deposits, err := database.ListDeposits(userID, 10)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Error fetching deposits")
		return
	}

	if len(deposits) == 0 {
		sendInfo(ctx, b, msg.Chat.ID, "📋 Recent Deposits", "No deposits found")
		return
	}

	var text strings.Builder
	text.WriteString("📋 <b>Recent Deposits</b>\n\n")

	username := msg.From.Username
	if username == "" {
		username = msg.From.FirstName
	}

	for i, deposit := range deposits {
		amount := float64(deposit.AmountRaw) / db.IVY_DECIMALS
		status := "❌ Pending"
		if deposit.Completed {
			status = "✅ Complete"
		}

		depositURL := fmt.Sprintf(
			"https://sprite.ivypowered.com/deposit?deposit_id=%s&user_id=%s&name=%s",
			deposit.DepositID,
			userID,
			url.QueryEscape(username),
		)

		text.WriteString(fmt.Sprintf("%d. %s <b>%.9f IVY</b>\n", i+1, status, amount))
		text.WriteString(fmt.Sprintf("   ID: <code>%s</code>\n", deposit.DepositID[:8]+"..."))
		text.WriteString(fmt.Sprintf("   <a href=\"%s\">Deposit Link</a>\n\n", depositURL))
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
