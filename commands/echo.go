package commands

import (
	"database/sql"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func EchoCommand(db *sql.DB, args []string, s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, strings.Join(args, " "))
}
