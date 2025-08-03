// discord/withdraw.go
package discord

import (
	"encoding/hex"
	"fmt"
	"net/url"

	"github.com/bwmarrin/discordgo"
	"github.com/gagliardetto/solana-go"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func getWithdrawUrl(withdrawId, discordId, discordName, signature string) string {
	return fmt.Sprintf(
		"https://sprite.ivypowered.com/withdraw?withdraw_id=%s&user_id=%s&name=%s&authority=%s&signature=%s",
		withdrawId,
		discordId,
		url.QueryEscape(discordName),
		constants.WITHDRAW_AUTHORITY_B58,
		signature,
	)
}

func WithdrawCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID != "" {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Withdrawals can only be processed in DMs for security. Please send this command directly to me.")
		return
	}

	if len(args) == 0 || args[0] != "list" && len(args) != 2 {
		DmUsage(s, m.Author.ID, "$withdraw amount sol_address OR $withdraw list", "Withdraw coins from your account or list past withdrawals. Must be used in DMs.\nExample: $withdraw 0.5 A32dqo7aTp3eHhxpSA6Cw67zWosKc3ymiYz2DbPVx8BK")
		return
	}

	if args[0] == "list" {
		listWithdrawals(database, s, m)
		return
	}

	amount, err := util.ParseAmount(args[0])
	if err != nil || amount <= 0 {
		DmError(s, m.Author.ID, "Please enter a valid positive amount")
		return
	}

	// Convert to RAW
	amountRaw := uint64(amount * constants.IVY_FACTOR)

	userKey, err := solana.PublicKeyFromBase58(args[1])
	if err != nil {
		DmError(s, m.Author.ID, "Please enter a valid base58-encoded Solana address")
		return
	}

	database.EnsureUserExists(m.Author.ID)

	balanceRaw, err := database.GetUserBalanceRaw(m.Author.ID)
	if err != nil {
		DmError(s, m.Author.ID, "Error checking balance")
		return
	}

	if balanceRaw < amountRaw {
		balance := float64(balanceRaw) / constants.IVY_FACTOR
		DmError(s, m.Author.ID, fmt.Sprintf("Insufficient balance. Your balance: **%.9f** IVY", balance))
		return
	}

	// Generate withdrawal ID
	withdrawIDBytes := util.GenerateID(amountRaw)
	withdrawID := hex.EncodeToString(withdrawIDBytes[:])

	// Sign withdrawal
	signature := util.SignWithdrawal(constants.SPRITE_VAULT, userKey, withdrawIDBytes, constants.WITHDRAW_AUTHORITY_KEY)
	signatureHex := hex.EncodeToString(signature[:])

	// Create withdrawal and debit user atomically
	err = database.CreateWithdrawal(withdrawID, m.Author.ID, balanceRaw, amountRaw, signatureHex)
	if err != nil {
		DmError(s, m.Author.ID, fmt.Sprintf("Error processing withdrawal: %v", err))
		return
	}

	// Get new balance
	newBalanceRaw, _ := database.GetUserBalanceRaw(m.Author.ID)
	newBalance := float64(newBalanceRaw) / constants.IVY_FACTOR

	// Get user info
	user, err := s.User(m.Author.ID)
	if err != nil {
		DmError(s, m.Author.ID, "Error getting user info")
		return
	}

	// Create withdrawal URL
	withdrawURL := getWithdrawUrl(
		withdrawID,
		m.Author.ID,
		user.Username,
		signatureHex,
	)

	// Send success embed
	embed := &discordgo.MessageEmbed{
		Title: "\U00002934 Withdrawal Created",
		Color: constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Amount",
				Value:  fmt.Sprintf("%.9f IVY", amount),
				Inline: true,
			},
			{
				Name:   "New Balance",
				Value:  fmt.Sprintf("%.9f IVY", newBalance),
				Inline: true,
			},
			{
				Name:   "Withdrawal ID",
				Value:  fmt.Sprintf("`%s`", withdrawID[:8]+"..."),
				Inline: false,
			},
			{
				Name:   "Claim Link",
				Value:  fmt.Sprintf("[Click here to claim withdrawal](%s)", withdrawURL),
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

func listWithdrawals(database db.Database, s *discordgo.Session, m *discordgo.MessageCreate) {
	// This requires adding a method to Database for listing withdrawals
	withdrawals, err := database.ListWithdrawals(m.Author.ID, 10)
	if err != nil {
		DmError(s, m.Author.ID, "Error fetching withdrawals")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:  "Recent Withdrawals",
		Color:  constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{},
	}

	if len(withdrawals) == 0 {
		embed.Description = "No withdrawals found"
	} else {
		for _, withdrawal := range withdrawals {
			amount := float64(withdrawal.AmountRaw) / constants.IVY_FACTOR

			// Get user info for URL
			user, _ := s.User(m.Author.ID)
			withdrawURL := getWithdrawUrl(
				withdrawal.WithdrawID,
				m.Author.ID,
				user.Username,
				withdrawal.Signature,
			)

			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   fmt.Sprintf("%.9f IVY", amount),
				Value:  fmt.Sprintf("ID: `%s`\n[Claim Link](%s)", withdrawal.WithdrawID[:8]+"...", withdrawURL),
				Inline: false,
			})
		}
	}

	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		return
	}
	s.ChannelMessageSendEmbed(channel.ID, embed)
}
