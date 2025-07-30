package telegram

import (
	"context"
	"fmt"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func IdCommand(ctx context.Context, b *bot.Bot, msg *models.Message) {
	id := getDatabaseID(msg.From.ID)
	response := fmt.Sprintf(
		`Your Ivy Sprite ID is **%s**.
In Discord, you can type

<code>$tip %s [amount]</code>

To transfer Ivy from Discord to Telegram!`,
		id,
		id,
	)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      response,
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		log.Println(err.Error())
	}
}
