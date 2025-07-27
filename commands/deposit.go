package commands

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func DepositCommand(db *sql.DB, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	if len(args) == 0 {
		util.DmUsage(s, m.Author.ID, "$deposit amount OR $deposit check id", "Create a new deposit or check an existing one\nExample: $deposit 0.75\nExample: $deposit check 3a8fb7")
		return
	}

	// Check if this is a deposit check
	if args[0] == "check" {
		if len(args) != 2 {
			util.DmUsage(s, m.Author.ID, "$deposit check <deposit_id>", "Check the status of a pending deposit")
			return
		}
		checkDeposit(db, args[1], s, m)
		return
	}

	if args[0] == "list" {
		listDeposits(db, s, m)
		return
	}

	// Parse amount for new deposit
	amount, err := strconv.ParseFloat(args[0], 64)
	if err != nil || amount <= 0 {
		util.DmError(s, m.Author.ID, "Please enter a valid positive amount")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * IVY_DECIMALS)

	ensureUserExists(db, m.Author.ID)

	// Generate deposit ID
	depositIDBytes := util.GenerateID(amountRaw)
	depositID := hex.EncodeToString(depositIDBytes[:])

	// Create deposit record
	err = createDeposit(db, depositID, m.Author.ID, amountRaw)
	if err != nil {
		util.DmError(s, m.Author.ID, "Error creating deposit")
		return
	}

	// Get user info
	user, err := s.User(m.Author.ID)
	if err != nil {
		util.DmError(s, m.Author.ID, "Error getting user info")
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

func checkDeposit(db *sql.DB, depositIDPrefix string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Find matching deposit
	var fullDepositID string
	var amountRaw uint64
	var completed int
	// If there's duplicate deposits, we select the latest one!
	err := db.QueryRow(
		"SELECT deposit_id, amount_raw, completed FROM deposits WHERE user_id = ? AND deposit_id LIKE ? ORDER BY timestamp DESC LIMIT 1",
		m.Author.ID,
		depositIDPrefix+"%",
	).Scan(&fullDepositID, &amountRaw, &completed)

	if err == sql.ErrNoRows {
		util.DmError(s, m.Author.ID, "No deposit found with that ID")
		return
	} else if err != nil {
		util.DmError(s, m.Author.ID, fmt.Sprintf("Error checking deposit: %v", err))
		return
	}

	if completed == 1 {
		util.DmSuccess(s, m.Author.ID, "This deposit has already been completed!", "Deposit Already Processed", "")
		return
	}

	// Decode deposit ID
	depositIDBytes, err := hex.DecodeString(fullDepositID)
	if err != nil || len(depositIDBytes) != 32 {
		util.DmError(s, m.Author.ID, "Invalid deposit ID format")
		return
	}
	var depositID32 [32]byte
	copy(depositID32[:], depositIDBytes)

	// Check if deposit is complete on-chain
	isComplete, err := util.IsDepositComplete(constants.RPC_CLIENT, constants.SPRITE_VAULT, depositID32)
	if err != nil {
		util.DmError(s, m.Author.ID, fmt.Sprintf("Error checking deposit status: %v", err))
		return
	}

	if !isComplete {
		util.DmClock(s, m.Author.ID, "Deposit incomplete", "Backend says deposit `"+fullDepositID[:8]+"...` is incomplete, try again!")
		return
	}

	// Complete the deposit
	err = completeDeposit(db, fullDepositID)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error completing deposit")
		return
	}

	// Get new balance
	newBalanceRaw, _ := getUserBalanceRaw(db, m.Author.ID)
	newBalance := float64(newBalanceRaw) / IVY_DECIMALS
	amount := float64(amountRaw) / IVY_DECIMALS

	util.DmSuccess(s, m.Author.ID,
		fmt.Sprintf("Deposited `%.9f IVY`\nNew balance: `%.9f IVY`", amount, newBalance),
		"Deposit complete",
		"")
}

func listDeposits(db *sql.DB, s *discordgo.Session, m *discordgo.MessageCreate) {
	rows, err := db.Query(
		"SELECT deposit_id, amount_raw, completed, timestamp FROM deposits WHERE user_id = ? ORDER BY timestamp DESC LIMIT 10",
		m.Author.ID,
	)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error fetching deposits")
		return
	}
	defer rows.Close()

	embed := &discordgo.MessageEmbed{
		Title:  "Recent Deposits",
		Color:  0x34D399,
		Fields: []*discordgo.MessageEmbedField{},
	}

	hasDeposits := false
	for rows.Next() {
		var depositID string
		var amountRaw uint64
		var completed int
		var timestamp int64
		rows.Scan(&depositID, &amountRaw, &completed, &timestamp)
		hasDeposits = true

		amount := float64(amountRaw) / IVY_DECIMALS
		status := "❌ Pending"
		if completed == 1 {
			status = "✅ Complete"
		}

		// Get user info for URL
		user, _ := s.User(m.Author.ID)
		depositURL := fmt.Sprintf(
			"https://sprite.ivypowered.com/deposit?deposit_id=%s&user_id=%s&name=%s",
			depositID,
			m.Author.ID,
			url.QueryEscape(user.Username),
		)

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s %.9f IVY", status, amount),
			Value:  fmt.Sprintf("ID: `%s`\n[Deposit Link](%s)", depositID[:8]+"...", depositURL),
			Inline: false,
		})
	}

	if !hasDeposits {
		embed.Description = "No deposits found"
	}

	util.ReactOk(s, m)
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		return
	}
	s.ChannelMessageSendEmbed(channel.ID, embed)
}
