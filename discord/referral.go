// discord/referral.go
package discord

import (
	"fmt"
	"net/url"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

const REFERRAL_USAGE = "$referral"
const REFERRAL_DETAILS = "Generate a referral link for the current contest using your most recently linked wallet"

func ReferralCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ensure user exists
	database.EnsureUserExists(m.Author.ID)

	// Get contest address
	contestAddress, err := database.GetContestAddress()
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Failed to retrieve contest address.")
		return
	}

	if contestAddress == "" {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "No contest is currently active. Ask Violet to set one!")
		return
	}

	// Get user's linked wallets
	wallets, err := database.GetUserWallets(m.Author.ID)
	if err != nil {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Error fetching your linked wallets")
		return
	}

	if len(wallets) == 0 {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "You haven't linked any wallets yet. Use `$link <wallet>` to link a wallet first.")
		return
	}

	// Use the most recently linked wallet (last in the list)
	referrerWallet := wallets[len(wallets)-1]

	// Generate referral URL
	referralURL := fmt.Sprintf(
		"https://ivypowered.com/game?address=%s&referrer=%s",
		url.QueryEscape(contestAddress),
		url.QueryEscape(referrerWallet),
	)

	// Create success embed
	embed := &discordgo.MessageEmbed{
		Title: "🔗 Your Referral Link",
		Color: constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Contest",
				Value:  fmt.Sprintf("`%s`", contestAddress),
				Inline: false,
			},
			{
				Name:   "Your Wallet",
				Value:  fmt.Sprintf("`%s`", referrerWallet),
				Inline: false,
			},
			{
				Name:   "Referral Link",
				Value:  referralURL,
				Inline: false,
			},
			{
				Name:   "How it works",
				Value:  "Share this link with friends! When they play using your referral link, you'll earn credit for their trading volume in the contest.",
				Inline: false,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Using wallet: %s", referrerWallet),
		},
	}

	ReactOk(s, m)

	// Send via DM
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		return
	}
	s.ChannelMessageSendEmbed(channel.ID, embed)
}
