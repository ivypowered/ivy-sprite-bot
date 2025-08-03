package discord

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/gagliardetto/solana-go"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

const LINK_USAGE = "$link <wallet> OR $link complete <response> OR $link list OR $link remove <wallet>"
const LINK_DETAILS = `Link your Solana wallet to your Discord account.

Commands:
• $link <wallet> - Generate a wallet linking URL for the specified wallet
• $link complete <response> - Complete wallet linking with the response
• $link list - Show your linked wallets
• $link remove <wallet> - Remove a linked wallet

Example flow:
1. Run $link YourWalletAddressHere
2. Visit the URL and sign with your wallet
3. Copy the response and run $link complete <response>`

func LinkCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID != "" {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Links can only be processed in DMs for security. Please send this command directly to me.")
		return
	}

	// Ensure user exists
	database.EnsureUserExists(m.Author.ID)

	if len(args) == 0 {
		DmUsage(s, m.Author.ID, LINK_USAGE, LINK_DETAILS)
		return
	}

	// Handle subcommands
	switch args[0] {
	case "complete":
		if len(args) != 2 {
			DmUsage(s, m.Author.ID, "$link complete <response>", "Complete wallet linking with the response from the website")
			return
		}
		verifyAndLink(database, args[1], s, m)
		return

	case "list":
		listWallets(database, s, m)
		return

	case "remove":
		if len(args) != 2 {
			DmUsage(s, m.Author.ID, "$link remove <wallet>", "Remove a linked wallet")
			return
		}
		removeWallet(database, args[1], s, m)
		return

	default:
		// Assume it's a wallet address
		generateLinkURL(args[0], s, m)
		return
	}
}

func generateLinkURL(walletStr string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Validate wallet address
	wallet, err := solana.PublicKeyFromBase58(walletStr)
	if err != nil {
		DmUsage(s, m.Author.ID, LINK_USAGE, LINK_DETAILS)
		return
	}

	linkURL := util.LinkGenerateURL(wallet, m.Author.ID)

	embed := &discordgo.MessageEmbed{
		Title: "🔗 Link Your Wallet",
		Color: constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Wallet",
				Value:  fmt.Sprintf("`%s`", walletStr),
				Inline: false,
			},
			{
				Name:   "Step 1",
				Value:  "Click the link below and connect the specified wallet",
				Inline: false,
			},
			{
				Name:   "Step 2",
				Value:  "Sign the message with your wallet",
				Inline: false,
			},
			{
				Name:   "Step 3",
				Value:  "Copy and return the provided `$link complete` command",
				Inline: false,
			},
			{
				Name:   "Link URL",
				Value:  fmt.Sprintf("[Click here to link wallet](%s)", linkURL),
				Inline: false,
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "This link expires in " + strconv.Itoa(util.LINK_RESPONSE_VALIDITY_INTERVAL/60) + " minutes",
		},
	}

	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		return
	}
	s.ChannelMessageSendEmbed(channel.ID, embed)
}

func verifyAndLink(database db.Database, responseBase64 string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Remove any whitespace
	responseBase64 = strings.TrimSpace(responseBase64)

	// Decode hex response
	responseBytes, err := base64.StdEncoding.DecodeString(responseBase64)
	if err != nil || len(responseBytes) != 104 {
		DmError(s, m.Author.ID, "Invalid response format. Please copy the entire response from the website.")
		return
	}

	// Convert to [104]byte
	var response [104]byte
	copy(response[:], responseBytes)

	// Verify the signature
	wallet, err := util.LinkVerify(response, m.Author.ID)
	if err != nil {
		DmError(s, m.Author.ID, fmt.Sprintf("Failed to verify signature: %v", err))
		return
	}

	// Link the wallet
	walletStr := solana.PublicKey(wallet).String()
	err = database.LinkWallet(walletStr, m.Author.ID)
	if err != nil {
		DmError(s, m.Author.ID, fmt.Sprintf("Failed to link wallet: %v", err))
		return
	}

	DmSuccess(s, m.Author.ID,
		fmt.Sprintf("Successfully linked wallet:\n`%s`", walletStr),
		"Wallet Linked",
		"")
}

func listWallets(database db.Database, s *discordgo.Session, m *discordgo.MessageCreate) {
	wallets, err := database.GetUserWallets(m.Author.ID)
	if err != nil {
		DmError(s, m.Author.ID, "Error fetching wallets")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: "Your Linked Wallets",
		Color: constants.IVY_GREEN,
	}

	if len(wallets) == 0 {
		embed.Description = "No wallets linked yet. Use `$link <wallet>` to link a wallet."
	} else {
		description := ""
		for i, wallet := range wallets {
			description += fmt.Sprintf("%d. `%s`\n", i+1, wallet)
		}
		embed.Description = description
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%d wallet(s) linked", len(wallets)),
		}
	}

	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		return
	}
	s.ChannelMessageSendEmbed(channel.ID, embed)
}

func removeWallet(database db.Database, wallet string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Validate wallet address
	_, err := solana.PublicKeyFromBase58(wallet)
	if err != nil {
		DmError(s, m.Author.ID, "Invalid wallet address format")
		return
	}

	err = database.UnlinkWallet(wallet, m.Author.ID)
	if err != nil {
		DmError(s, m.Author.ID, fmt.Sprintf("Failed to remove wallet: %v", err))
		return
	}

	DmSuccess(s, m.Author.ID,
		fmt.Sprintf("Successfully removed wallet:\n`%s`", wallet),
		"Wallet Removed",
		"")
}
