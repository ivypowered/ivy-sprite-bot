package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ivypowered/ivy-sprite-bot/commands"
	"github.com/ivypowered/ivy-sprite-bot/constants"

	"github.com/bwmarrin/discordgo"
	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

var DISCORD_TOKEN string = os.Getenv("DISCORD_TOKEN")

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./bot.db")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS users (
            user_id TEXT PRIMARY KEY,
            balance_raw INTEGER DEFAULT 0
        );
    `)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS deposits (
            deposit_id TEXT PRIMARY KEY,
            user_id TEXT NOT NULL,
            timestamp INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
            amount_raw INTEGER NOT NULL,
            completed INTEGER NOT NULL DEFAULT 0
        );
    `)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_deposit_user ON deposits(user_id);
    `)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS withdrawals (
            withdraw_id TEXT PRIMARY KEY,
            user_id TEXT NOT NULL,
            timestamp INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
            amount_raw INTEGER NOT NULL,
            signature TEXT NOT NULL
        );
    `)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_withdrawal_user ON withdrawals(user_id);
    `)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS activity (
            server_id TEXT NOT NULL,
            user_id TEXT NOT NULL,
            score INTEGER DEFAULT 1,
            last_message_timestamp INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
            PRIMARY KEY (server_id, user_id)
        );
    `)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_activity_server ON activity(server_id);
    `)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func main() {
	// Initialize database
	var err error
	DB, err = initDB()
	if err != nil {
		log.Fatal("Error initializing database:", err)
	}
	defer DB.Close()

	// Queue initial price update
	go constants.PRICE.Update(constants.RPC_CLIENT)

	// Register all commands
	registerAllCommands()

	// Create Discord session
	if DISCORD_TOKEN == "" {
		log.Fatal("DISCORD_TOKEN environment variable not set")
	}

	dg, err := discordgo.New("Bot " + DISCORD_TOKEN)
	if err != nil {
		log.Fatal("Error creating Discord session:", err)
	}

	// Register message handler
	dg.AddHandler(messageHandler)

	// Set intents
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	// Open websocket connection
	err = dg.Open()
	if err != nil {
		log.Fatal("Error opening connection:", err)
	}

	log.Println("bot online; send SIGINT to exit")

	// Wait for interrupt signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	log.Println("closing WS connection...")
	dg.Close()
}

func registerAllCommands() {
	RegisterCommand("tip", commands.TipCommand)
	RegisterCommand("deposit", commands.DepositCommand)
	RegisterCommand("withdraw", commands.WithdrawCommand)
	RegisterCommand("balance", commands.BalanceCommand)
	RegisterCommand("help", commands.HelpCommand)
	RegisterCommand("echo", commands.EchoCommand)
	RegisterCommand("rain", commands.RainCommand)
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore bot messages
	if m.Author.Bot {
		return
	}

	content := strings.TrimSpace(m.Content)

	// Check if message starts with $
	if !strings.HasPrefix(content, "$") {
		// Only track activity in guild channels (not DMs)
		if m.GuildID != "" {
			// Update activity score for this user
			err := updateActivityScore(DB, m.GuildID, m.Author.ID)
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
	if cmdFunc, exists := GetCommand(cmdName); exists {
		cmdFunc(DB, args, s, m)
	}
}
