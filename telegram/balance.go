package telegram

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

func BalanceCommand(ctx context.Context, database db.Database, b *bot.Bot, msg *models.Message) {
	id := getDatabaseID(msg.From.ID)
	database.EnsureUserExists(id)

	balanceRaw, err := database.GetUserBalanceRaw(id)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, err.Error())
		return
	}

	// Convert RAW to display value
	balance := float64(balanceRaw) / db.IVY_DECIMALS
	price := constants.PRICE.Get(constants.RPC_CLIENT)

	// Format the balance message
	name := msg.From.FirstName
	if msg.From.Username != "" {
		name = "@" + msg.From.Username
	}

	text := fmt.Sprintf(`<b>%s's Ivy Wallet</b>

<b>Balance</b>
├ 🌿 %.9f IVY
└ 💵 ≈ $%.2f USD

📊 <b>Current Price</b>
└ $%.4f per IVY`,
		escapeHTML(name),
		balance,
		balance*price,
		price)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}
