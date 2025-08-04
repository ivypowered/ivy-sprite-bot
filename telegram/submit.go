// telegram/submit.go
package telegram

import (
	"context"
	"fmt"
	"net/url"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func SubmitCommand(ctx context.Context, b *bot.Bot, msg *models.Message, args []string, submitC chan<- string) {
	// Check if user provided an argument
	if len(args) == 0 {
		sendUsage(ctx, b, msg.Chat.ID, "/submit", `Submit a game link to the Ivy Discord

<b>Usage:</b>
• /submit [game_link] - Submit a sprite game link

<b>Example:</b>
• /submit https://scratch.mit.edu/projects/1201692556/

<b>Note:</b>
• Your submission will be posted in the Ivy Discord`)
		return
	}

	// Get the link
	gameLink := args[0]

	// Validate URL
	_, err := url.Parse(gameLink)
	if err != nil {
		sendError(ctx, b, msg.Chat.ID, "Invalid URL format. Please provide a valid game link.")
		return
	}

	// Get submitter info
	submitterName := msg.From.FirstName
	if msg.From.Username != "" {
		submitterName = "@" + msg.From.Username
	}

	// Format the message for Discord
	discordMessage := fmt.Sprintf("from Telegram:\n\n*Submitted by:* %s\n*Game Link:* %s",
		submitterName,
		gameLink)

	// Send to Discord channel via the submit channel
	select {
	case submitC <- discordMessage:
		// Success - send confirmation to user
		sendSuccess(ctx, b, msg.Chat.ID,
			fmt.Sprintf("Your game link has been submitted to the Ivy Discord!\n\n<b>Link:</b> %s", escapeHTML(gameLink)),
			"✅ Game Submitted")
	default:
		// Channel might be full or closed
		sendError(ctx, b, msg.Chat.ID, "Failed to submit game link. Please try again later.")
	}
}
