package discord

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

type CommandFunc func(
	db db.Database,
	args []string,
	s *discordgo.Session,
	m *discordgo.MessageCreate,
)

// starts the discord connection, returns a function that closes it!
func Start(db db.Database, token string) (func() error, error) {
	if token == "" {
		return nil, errors.New("no token passed to discord.Start")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("Error creating Discord session: %v", err)
	}

	commands := map[string]CommandFunc{
		"balance":  BalanceCommand,
		"deposit":  DepositCommand,
		"help":     HelpCommand,
		"id":       IdCommand,
		"rain":     RainCommand,
		"tip":      TipCommand,
		"withdraw": WithdrawCommand,
	}

	// Register message handler
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore bot messages
		if m.Author.Bot {
			return
		}

		content := strings.TrimSpace(m.Content)

		// If it's not a bot command
		if !strings.HasPrefix(content, "$") {
			// Only track activity in guild channels (not DMs)
			if m.GuildID == "" {
				return
			}
			// Check if this channel is whitelisted for rain
			isRainChannel, err := db.IsRainChannel(m.GuildID, m.ChannelID)
			if err != nil {
				log.Printf("Error checking rain channel: %v", err)
			} else if isRainChannel {
				// Update activity score only for whitelisted channels
				err := db.UpdateActivityScore(m.GuildID, m.Author.ID)
				if err != nil {
					log.Printf("Error updating activity score: %v", err)
				}
			}
			return
		}

		// Parse command and arguments
		parts := strings.Fields(content)
		if len(parts) == 0 {
			return
		}

		// Extract command name (remove $ prefix)
		cmdName := strings.TrimPrefix(parts[0], "$")
		args := parts[1:]

		// Look up and execute command
		if f, exists := commands[cmdName]; exists {
			f(db, args, s, m)
		}
	})

	// Set intents
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	// Open websocket connection
	err = dg.Open()
	if err != nil {
		return nil, fmt.Errorf("Error opening connection: %v", err)
	}

	// Return close fn
	return func() error {
		return dg.Close()
	}, nil
}
