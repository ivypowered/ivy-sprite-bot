package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

type CommandFunc func(
	database db.Database,
	b *bot.Bot,
	msg *models.Message,
	args []string,
)

func Start(database db.Database, token string) (func() error, error) {
	if token == "" {
		return nil, errors.New("no token passed to telegram.Start")
	}

	// Handler function
	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}

		msg := update.Message
		if msg.From == nil {
			return
		}

		// Ignore bot messages
		if msg.From.IsBot {
			return
		}

		text := strings.TrimSpace(msg.Text)

		// Parse command
		if !strings.HasPrefix(text, "/") {
			// not IVY server?
			if msg.Chat.ID != IVY_TELEGRAM_CHANNEL_ID {
				return
			}
			err := database.UpdateActivityScore("telegram", getDatabaseID(msg.From.ID))
			if err != nil {
				log.Printf("error updating TG activity score: %v\n", err)
			}
			return
		}

		parts := strings.Fields(text)
		if len(parts) == 0 {
			return
		}

		// Extract command name
		command := strings.TrimPrefix(parts[0], "/")
		command = strings.Split(command, "@")[0] // Remove bot username if present
		args := parts[1:]

		// Route commands
		switch command {
		case "start", "help":
			HelpCommand(ctx, b, msg)
		case "move":
			MoveCommand(ctx, database, b, msg, args)
		case "id":
			IdCommand(ctx, b, msg)
		case "balance":
			BalanceCommand(ctx, database, b, msg)
		case "deposit":
			DepositCommand(ctx, database, b, msg, args)
		case "withdraw":
			WithdrawCommand(ctx, database, b, msg, args)
		case "tip":
			TipCommand(ctx, database, b, msg, args)
		case "rain":
			RainCommand(ctx, database, b, msg, args)
		default:
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.Chat.ID,
				Text:   "Unknown command. Use /help to see available commands.",
			})
		}
	}

	// Create bot
	b, err := bot.New(token, bot.WithDefaultHandler(handler))
	if err != nil {
		return nil, fmt.Errorf("Error creating Telegram bot: %v", err)
	}

	// Set up commands
	_, err = b.SetMyCommands(context.Background(), &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "balance", Description: "Check your Ivy balance"},
			{Command: "deposit", Description: "Deposit Ivy tokens (Private chat only)"},
			{Command: "withdraw", Description: "Withdraw Ivy tokens (Private chat only)"},
			{Command: "tip", Description: "Tip Ivy tokens to another user"},
			{Command: "rain", Description: "Rain Ivy tokens on active users"},
			{Command: "id", Description: "See your Ivy Sprite ID"},
			{Command: "help", Description: "Show available commands"},
			{Command: "move", Description: "Move funds to Discord (Private chat only)"},
		},
	})
	if err != nil {
		return nil, err
	}

	// Start bot
	ctx, cancelFn := context.WithCancel(context.Background())
	go b.Start(ctx)

	log.Println("Telegram bot started successfully")

	// Return close function
	return func() error {
		cancelFn()
		return nil
	}, nil
}
