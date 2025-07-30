package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/discord"
	"github.com/ivypowered/ivy-sprite-bot/telegram"
)

var DISCORD_TOKEN string = os.Getenv("DISCORD_TOKEN")
var TELEGRAM_TOKEN string = os.Getenv("TELEGRAM_TOKEN")

func main() {
	// Initialize database
	var err error
	database, err := db.New("./bot.db")
	if err != nil {
		log.Fatal("Error initializing database:", err)
	}
	defer database.Close()

	// Queue initial price update
	go constants.PRICE.Update(constants.RPC_CLIENT)

	// Track cleanup functions
	var cleanupFuncs []func() error

	// Start Telegram bot if token is provided
	stopTelegramFn, err := telegram.Start(database, TELEGRAM_TOKEN)
	if err != nil {
		log.Fatal("Error starting Telegram bot:", err)
	}
	cleanupFuncs = append(cleanupFuncs, stopTelegramFn)
	log.Println("Telegram bot online")

	// Start Discord bot
	stopDiscordFn, err := discord.Start(database, DISCORD_TOKEN)
	if err != nil {
		log.Fatal("Error starting Discord bot:", err)
	}
	cleanupFuncs = append(cleanupFuncs, stopDiscordFn)
	log.Println("Discord bot online")

	log.Println("Send SIGINT to exit")

	// Wait for interrupt signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanup all bots
	log.Println("Shutting down...")
	for _, cleanup := range cleanupFuncs {
		if err := cleanup(); err != nil {
			log.Printf("Error during cleanup: %v", err)
		}
	}
	log.Println("Shutdown complete")
}
