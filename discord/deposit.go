// discord/deposit.go
package discord

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func DepositCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID != "" {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Withdrawals can only be processed in DMs for security. Please send this command directly to me.")
		return
	}

	if len(args) == 0 {
		DmUsage(s, m.Author.ID, "$deposit amount OR $deposit check id", "Create a new deposit or check an existing one\nExample: $deposit 0.75\nExample: $deposit check 3a8fb7")
		return
	}

	// Check if this is a deposit check
	if args[0] == "check" {
		if len(args) != 2 {
			DmUsage(s, m.Author.ID, "$deposit check <deposit_id>", "Check the status of a pending deposit")
			return
		}
		checkDeposit(database, args[1], s, m)
		return
	}

	if args[0] == "list" {
		listDeposits(database, s, m)
		return
	}

	// Parse amount for new deposit
	amount, err := strconv.ParseFloat(args[0], 64)
	if err != nil || amount <= 0 {
		DmError(s, m.Author.ID, "Please enter a valid positive amount")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * constants.IVY_FACTOR)

	database.EnsureUserExists(m.Author.ID)

	// Generate deposit ID
	depositIDBytes := util.GenerateID(amountRaw)
	depositID := hex.EncodeToString(depositIDBytes[:])

	// Create deposit record
	err = database.CreateDeposit(depositID, m.Author.ID, amountRaw)
	if err != nil {
		DmError(s, m.Author.ID, "Error creating deposit")
		return
	}

	// Get user info
	user, err := s.User(m.Author.ID)
	if err != nil {
		DmError(s, m.Author.ID, "Error getting user info")
		return
	}

	// Create deposit URL
	depositURL := fmt.Sprintf(
		"https://sprite.ivypowered.com/deposit?deposit_id=%s&user_id=%s&name=%s",
		depositID,
		m.Author.ID,
		url.QueryEscape(user.Username),
	)

	// Send success embed
	embed := &discordgo.MessageEmbed{
		Title: "\U00002935 Deposit created",
		Color: constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Amount",
				Value:  fmt.Sprintf("`%.9f IVY`", amount),
				Inline: true,
			},
			{
				Name:   "Deposit ID",
				Value:  fmt.Sprintf("`%s`", depositID[:8]+"..."),
				Inline: true,
			},
			{
				Name:   "Instructions",
				Value:  "1. Click the link below to complete your deposit\n2. After sending, use `$deposit check " + depositID[:6] + "` to verify",
				Inline: false,
			},
			{
				Name:   "Deposit Link",
				Value:  fmt.Sprintf("[Click here to deposit](%s)", depositURL),
				Inline: false,
			},
		},
	}

	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		return
	}
	s.ChannelMessageSendEmbed(channel.ID, embed)
}

func checkDeposit(database db.Database, depositIDPrefix string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Find matching deposit - need to access inner DB for this query
	var fullDepositID string
	var amountRaw uint64
	var completed int
	// If there's duplicate deposits, we select the latest one!
	// Note: This requires exposing the inner DB or adding a method to Database
	// For now, let's add a method to Database for this specific query
	fullDepositID, amountRaw, completed, err := database.FindDepositByPrefix(m.Author.ID, depositIDPrefix)

	if err == sql.ErrNoRows {
		DmError(s, m.Author.ID, "No deposit found with that ID")
		return
	} else if err != nil {
		DmError(s, m.Author.ID, fmt.Sprintf("Error checking deposit: %v", err))
		return
	}

	if completed == 1 {
		DmSuccess(s, m.Author.ID, "This deposit has already been completed!", "Deposit Already Processed", "")
		return
	}

	// Decode deposit ID
	depositIDBytes, err := hex.DecodeString(fullDepositID)
	if err != nil || len(depositIDBytes) != 32 {
		DmError(s, m.Author.ID, "Invalid deposit ID format")
		return
	}
	var depositID32 [32]byte
	copy(depositID32[:], depositIDBytes)

	// Check if deposit is complete on-chain
	isComplete, err := util.IsDepositComplete(constants.RPC_CLIENT, constants.SPRITE_VAULT, depositID32)
	if err != nil {
		DmError(s, m.Author.ID, fmt.Sprintf("Error checking deposit status: %v", err))
		return
	}

	if !isComplete {
		DmClock(s, m.Author.ID, "Deposit incomplete", "Backend says deposit `"+fullDepositID[:8]+"...` is incomplete, try again!")
		return
	}

	// Complete the deposit
	err = database.CompleteDeposit(fullDepositID)
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Error completing deposit")
		return
	}

	// Get new balance
	newBalanceRaw, _ := database.GetUserBalanceRaw(m.Author.ID)
	newBalance := float64(newBalanceRaw) / constants.IVY_FACTOR
	amount := float64(amountRaw) / constants.IVY_FACTOR

	DmSuccess(s, m.Author.ID,
		fmt.Sprintf("Deposited `%.9f IVY`\nNew balance: `%.9f IVY`", amount, newBalance),
		"Deposit complete",
		"")
}

func listDeposits(database db.Database, s *discordgo.Session, m *discordgo.MessageCreate) {
	// This requires adding a method to Database for listing deposits
	deposits, err := database.ListDeposits(m.Author.ID, 10)
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Error fetching deposits")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:  "Recent Deposits",
		Color:  0x34D399,
		Fields: []*discordgo.MessageEmbedField{},
	}

	if len(deposits) == 0 {
		embed.Description = "No deposits found"
	} else {
		for _, deposit := range deposits {
			amount := float64(deposit.AmountRaw) / constants.IVY_FACTOR
			status := "❌ Pending"
			if deposit.Completed {
				status = "✅ Complete"
			}

			// Get user info for URL
			user, _ := s.User(m.Author.ID)
			depositURL := fmt.Sprintf(
				"https://sprite.ivypowered.com/deposit?deposit_id=%s&user_id=%s&name=%s",
				deposit.DepositID,
				m.Author.ID,
				url.QueryEscape(user.Username),
			)

			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   fmt.Sprintf("%s %.9f IVY", status, amount),
				Value:  fmt.Sprintf("ID: `%s`\n[Deposit Link](%s)", deposit.DepositID[:8]+"...", depositURL),
				Inline: false,
			})
		}
	}

	ReactOk(s, m)
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		return
	}
	s.ChannelMessageSendEmbed(channel.ID, embed)
}
