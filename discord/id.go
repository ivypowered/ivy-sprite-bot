// discord/id.go
package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

func IdCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	// Create embed with ID information
	embed := &discordgo.MessageEmbed{
		Title: "Your ID",
		Color: constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Discord ID",
				Value:  fmt.Sprintf("`%s`", m.Author.ID),
				Inline: false,
			},
			{
				Name:   "How to receive from Telegram",
				Value:  fmt.Sprintf("In Telegram, users can type:\n`/move [amount] %s`\n\nTo transfer IVY from Telegram to your Discord account!", m.Author.ID),
				Inline: false,
			},
		},
	}

	// Send via DM
	channel, err := s.UserChannelCreate(m.Author.ID)
	if err != nil {
		ReactErr(s, m)
		return
	}

	ReactOk(s, m)
	s.ChannelMessageSendEmbed(channel.ID, embed)
}
