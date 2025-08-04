package discord

import (
	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

func HelpCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Create help embed for DM
	embed := &discordgo.MessageEmbed{
		Title: "Commands",
		Color: constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Balance",
				Value:  "`$balance` - Check your current balance",
				Inline: false,
			},
			{
				Name:   "Tip",
				Value:  "`$tip @user <amount>` - Send coins to another user",
				Inline: false,
			},
			{
				Name:   "Deposit",
				Value:  "`$deposit <amount>` - Create a new deposit\n`$deposit check <id>` - Check deposit status\n`$deposit list` - List recent deposits",
				Inline: false,
			},
			{
				Name:   "Withdraw",
				Value:  "`$withdraw <amount>` - Withdraw coins\n`$withdraw list` - List recent withdrawals",
				Inline: false,
			},
			{
				Name:   "ID",
				Value:  "`$id` - Show your Discord ID for receiving transfers from Telegram",
				Inline: false,
			},
			{
				Name:   "Link",
				Value:  "`$link <wallet>` - Link a Solana wallet\n`$link complete <response>` - Complete wallet linking\n`$link list` - List linked wallets\n`$link remove <wallet>` - Remove a linked wallet",
				Inline: false,
			},
			{
				Name:   "Leaderboard",
				Value:  "`$leaderboard` - Show the leaderboard for the current contest",
				Inline: false,
			},
			{
				Name:   "Volume",
				Value:  "`$volume` - Show your total trading volume across all linked wallets",
				Inline: false,
			},
			{
				Name:   "Help",
				Value:  "`$help` - Show this help message",
				Inline: false,
			},
		},
	}

	// Send help via DM
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		return
	}
	s.ChannelMessageSendEmbed(channel.ID, embed)
}
