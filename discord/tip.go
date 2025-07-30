// discord/tip.go
package discord

import (
	"fmt"
	"regexp"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

var DISCORD_ID_REGEX = regexp.MustCompile(`<@!?(\d+)>`)
var TELEGRAM_ID_REGEX = regexp.MustCompile(`tg:(\d+)$`)

func TipCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	if len(args) != 2 {
		util.ReactErr(s, m)
		util.DmUsage(s, m.Author.ID, "$tip @user <amount>", "Send coins to another user. Mention the user and specify a positive amount.")
		return
	}

	// Extract user ID from mention
	discordMatches := DISCORD_ID_REGEX.FindStringSubmatch(args[0])
	tgMatches := TELEGRAM_ID_REGEX.FindStringSubmatch(args[0])
	var recipientID string
	if len(discordMatches) == 2 {
		recipientID = discordMatches[1]
		// Ensure Discord user exists
		database.EnsureUserExists(recipientID)
	} else if len(tgMatches) == 2 {
		recipientID = tgMatches[0]
		// Don't ensure TG user exists, because it might be wrong
		extant, err := database.IsUserExtant(recipientID)
		if err != nil {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, fmt.Sprintf("Error querying db: %v", err))
			return
		}
		if !extant {
			util.ReactErr(s, m)
			util.DmError(s, m.Author.ID, fmt.Sprintf("Telegram user %s not found, make sure they have run /balance at least once", recipientID))
			return
		}
	} else {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Please mention a valid user")
		return
	}

	// Parse amount
	amount, err := util.ParseAmount(args[1])
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Please enter a valid positive amount")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * db.IVY_DECIMALS)

	// Ensure author exists in database
	database.EnsureUserExists(m.Author.ID)

	// Check sender's balance
	senderBalanceRaw, err := database.GetUserBalanceRaw(m.Author.ID)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, "Error checking balance")
		return
	}

	if senderBalanceRaw < amountRaw {
		senderBalance := float64(senderBalanceRaw) / db.IVY_DECIMALS
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("Insufficient balance. Your balance: **%.9f** IVY", senderBalance))
		return
	}

	// Perform transfer
	err = database.TransferFundsRaw(m.Author.ID, recipientID, amountRaw)
	if err != nil {
		util.ReactErr(s, m)
		util.DmError(s, m.Author.ID, fmt.Sprintf("Error processing transfer: %v", err))
		return
	}

	// Get new balances
	newBalanceRaw, _ := database.GetUserBalanceRaw(m.Author.ID)
	newBalance := float64(newBalanceRaw) / db.IVY_DECIMALS

	util.ReactOk(s, m)

	// DM sender confirmation
	util.DmSuccess(s, m.Author.ID,
		fmt.Sprintf("Successfully sent **%.9f** IVY to <@%s>\n\nYour new balance: **%.9f** IVY", amount, recipientID, newBalance),
		"Transfer Complete",
		"")

	// DM recipient notification
	recipientBalanceRaw, _ := database.GetUserBalanceRaw(recipientID)
	recipientBalance := float64(recipientBalanceRaw) / db.IVY_DECIMALS
	util.DmSuccess(s, recipientID,
		fmt.Sprintf("You received **%.9f** IVY from <@%s>\n\nYour new balance: **%.9f** IVY", amount, m.Author.ID, recipientBalance),
		"Payment Received",
		"")
}
