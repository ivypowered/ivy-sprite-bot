package commands

import (
	"database/sql"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/util"
)

func BalanceCommand(db *sql.DB, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	ensureUserExists(db, m.Author.ID)

	balanceRaw, err := getUserBalanceRaw(db, m.Author.ID)
	if err != nil {
		util.DmError(s, m.Author.ID, err.Error())
		return
	}

	// Convert RAW to display value
	balance := float64(balanceRaw) / IVY_DECIMALS

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
