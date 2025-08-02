package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const IVY_TELEGRAM_CHANNEL_ID int64 = -1002894078752

// Convert tg id -> database id
func getDatabaseID(tgId int64) string {
	return fmt.Sprintf("tg:%d", tgId)
}

// Convert database id -> tg id
func fromDatabaseID(dbId string) (int64, error) {
	if !strings.HasPrefix(dbId, "tg:") {
		return 0, errors.New("db id lacking \"tg:\" prefix")
	}
	return strconv.ParseInt(dbId[3:], 10, 64)
}

// Helper functions for consistent message formatting

func sendError(ctx context.Context, b *bot.Bot, chatID int64, message string) {
	text := fmt.Sprintf("❌ <b>Error</b>\n\n%s", escapeHTML(message))
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

func sendSuccess(ctx context.Context, b *bot.Bot, chatID int64, message string, title string) {
	text := fmt.Sprintf("%s\n\n%s", title, message)
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

func sendUsage(ctx context.Context, b *bot.Bot, chatID int64, command string, details string) {
	text := fmt.Sprintf("📖 <b>Usage: %s</b>\n\n%s", escapeHTML(command), details)
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

func sendClock(ctx context.Context, b *bot.Bot, chatID int64, title string, message string) {
	text := fmt.Sprintf("⏳ <b>%s</b>\n\n%s", escapeHTML(title), message)
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

func sendInfo(ctx context.Context, b *bot.Bot, chatID int64, title string, message string) {
	text := fmt.Sprintf("%s\n\n%s", title, escapeHTML(message))
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

// Escape special HTML characters
func escapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
