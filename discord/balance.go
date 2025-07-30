// discord/balance.go
package discord

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func BalanceCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	database.EnsureUserExists(m.Author.ID)

	balanceRaw, err := database.GetUserBalanceRaw(m.Author.ID)
	if err != nil {
		util.DmError(s, m.Author.ID, err.Error())
		return
	}

	// Convert RAW to display value
	balance := float64(balanceRaw) / db.IVY_DECIMALS

	// Create the embed for DM
	name := m.Author.GlobalName
	if name == "" {
		name = m.Author.Username
	}
	price := constants.PRICE.Get(constants.RPC_CLIENT)
	embed := &discordgo.MessageEmbed{
		Color: constants.IVY_GREEN,
		Author: &discordgo.MessageEmbedAuthor{
			Name:    name + "'s Ivy wallet",
			IconURL: m.Author.AvatarURL("128"),
		},
		Description: fmt.Sprintf("**Balance**\n<:ivy:1398745198472986654> **%.9f IVY** (\U00002248 $%.2f)", balance, balance*price),
	}

	// Send balance via DM
	s.ChannelMessageSendEmbed(m.ChannelID, embed)
}
