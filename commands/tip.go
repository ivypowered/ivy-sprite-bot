package commands

import (
	"database/sql"
	"fmt"
	"regexp"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func TipCommand(db *sql.DB, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	if len(args) != 2 {
		util.ReactErr(s, m)
		util.DmUsage(s, m.Author.ID, "$tip @user <amount>", "Send coins to another user. Mention the user and specify a positive amount.")
		return
	}

	// Extract user ID from mention
	userIDRegex := regexp.MustCompile(`<@!?(\d+)>`)
	matches := userIDRegex.FindStringSubmatch(args[0])
	if len(matches) != 2 {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Please mention a valid user")
		return
	}
	recipientID := matches[1]

	// Parse amount
	amount, err := util.ParseAmount(args[1])
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Please enter a valid positive amount")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * IVY_DECIMALS)

	// Ensure both users exist in database
	ensureUserExists(db, m.Author.ID)
	ensureUserExists(db, recipientID)

	// Check sender's balance
	senderBalanceRaw, err := getUserBalanceRaw(db, m.Author.ID)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error checking balance")
		return
	}

	if senderBalanceRaw < amountRaw {
		senderBalance := float64(senderBalanceRaw) / IVY_DECIMALS
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("Insufficient balance. Your balance: **%.9f** IVY", senderBalance))
		return
	}

	// Perform transfer
	err = transferFundsRaw(db, m.Author.ID, recipientID, amountRaw)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error processing transfer")
		return
	}

	// Get new balances
	newBalanceRaw, _ := getUserBalanceRaw(db, m.Author.ID)
	newBalance := float64(newBalanceRaw) / IVY_DECIMALS

	util.ReactOk(s, m)

	// DM sender confirmation
	util.DmSuccess(s, m.Author.ID,
		fmt.Sprintf("Successfully sent **%.9f** IVY to <@%s>\n\nYour new balance: **%.9f** IVY", amount, recipientID, newBalance),
		"Transfer Complete",
		"")

	// DM recipient notification
	recipientBalanceRaw, _ := getUserBalanceRaw(db, recipientID)
	recipientBalance := float64(recipientBalanceRaw) / IVY_DECIMALS
	util.DmSuccess(s, recipientID,
		fmt.Sprintf("You received **%.9f** IVY from <@%s>\n\nYour new balance: **%.9f** IVY", amount, m.Author.ID, recipientBalance),
		"Payment Received",
		"")
}
