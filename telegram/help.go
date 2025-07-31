package telegram

import (
	"context"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func HelpCommand(ctx context.Context, b *bot.Bot, msg *models.Message) {
	helpText := `🌿 <b>Ivy Sprite Bot Commands</b>

💰 <b>Balance</b>
• /balance - Check your current balance

📥 <b>Deposit</b>
• /deposit [amount] - Create a new deposit
• /deposit check [id] - Check deposit status
• /deposit list - List recent deposits

📤 <b>Withdraw</b> <i>(Private chat only)</i>
• /withdraw [amount] [address] - Withdraw coins
• /withdraw list - List recent withdrawals

💸 <b>Tip</b>
• Reply to a message with /tip [amount] - Send coins to user

ℹ️ <b>Help</b>
• /help - Show this help message

💳 <b>ID</b>
• /id - Show the ID for this account, to transfer IVY from Discord to Telegram

🔄 <b>Move</b> <i>(Private chat only)</i>
• /move [amount] [discord_id] - Move funds to Discord

📝 <b>Examples:</b>
• /deposit 10.5
• /withdraw 5.0 YourSolanaAddress
• /tip $2.5`

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      helpText,
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		log.Println(err.Error())
	}
}
