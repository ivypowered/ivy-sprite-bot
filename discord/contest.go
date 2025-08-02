package discord

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/gagliardetto/solana-go"
	"github.com/ivypowered/ivy-sprite-bot/constants"
	"github.com/ivypowered/ivy-sprite-bot/db"
)

const CONTEST_USAGE = "$contest set <address>"
const CONTEST_DETAILS = "Set the contest game address (Violet only)"

func ContestCommand(database db.Database, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID != "" {
		ReactErr(s, m)
		DmError(s, m.Author.ID, "Contest command is DM only")
		return
	}
	// Check if user is Violet
	if m.Author.ID != constants.VIOLET_ID {
		DmError(s, m.Author.ID, "This command can only be used by Violet.")
		return
	}

	if len(args) < 2 || args[0] != "set" {
		DmUsage(s, m.Author.ID, CONTEST_USAGE, CONTEST_DETAILS)
		return
	}

	// Validate the address
	address := strings.TrimSpace(args[1])
	_, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		DmError(s, m.Author.ID, "Invalid Solana address format.")
		return
	}

	// Set the contest address
	err = database.SetContestAddress(address)
	if err != nil {
		DmError(s, m.Author.ID, "Failed to set contest address.")
		return
	}

	// Send success message
	embed := &discordgo.MessageEmbed{
		Title: "Contest Address Set",
		Color: constants.IVY_GREEN,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Game Address",
				Value:  "```" + address + "```",
				Inline: false,
			},
		},
	}
	s.ChannelMessageSendEmbed(m.Author.ID, embed)
}
