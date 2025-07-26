package util

import (
	"errors"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/ivypowered/ivy-sprite-bot/constants"
)

func ReactOk(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.MessageReactionAdd(m.ChannelID, m.ID, "\U00002705") // green check
}

func ReactClock(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.MessageReactionAdd(m.ChannelID, m.ID, "\U0001F552") // three o' clock
}

func ReactErr(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.MessageReactionAdd(m.ChannelID, m.ID, "\U0000274C") // x
}

// DmUsage sends a usage embed to a user via DM
func DmUsage(s *discordgo.Session, userID string, commandName string, commandDetails string) (*discordgo.Message, error) {
	// Create DM channel
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return nil, err
	}

	// Create purple embed
	embed := &discordgo.MessageEmbed{
		Title: "Usage",
		Color: constants.IVY_PURPLE, // Purple
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   commandName,
				Value:  commandDetails,
				Inline: false,
			},
		},
	}

	// Send the message
	msg, err := s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		// User may have blocked the bot
		return nil, err
	}

	return msg, nil
}

// DmError sends an error embed to a user via DM
func DmError(s *discordgo.Session, userID string, message string) (*discordgo.Message, error) {
	// Create DM channel
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return nil, err
	}

	// Create red embed
	embed := &discordgo.MessageEmbed{
		Title:       "Error",
		Description: message,
		Color:       constants.IVY_RED, // Red
	}

	// Send the message
	msg, err := s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		// User may have blocked the bot
		return nil, err
	}

	return msg, nil
}

// DmClock sends an clock embed to a user via DM
func DmClock(s *discordgo.Session, userID string, title string, message string) (*discordgo.Message, error) {
	// Create DM channel
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return nil, err
	}

	// Create white embed
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: message,
		Color:       constants.IVY_WHITE, // White
	}

	// Send the message
	msg, err := s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		// User may have blocked the bot
		return nil, err
	}

	return msg, nil
}

// DmSuccess sends a success embed to a user via DM
func DmSuccess(s *discordgo.Session, userID string, message string, header string, footer string) (*discordgo.Message, error) {
	// Create DM channel
	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		return nil, err
	}

	// Use default header if empty
	if header == "" {
		header = "Success"
	}

	// Create green embed
	embed := &discordgo.MessageEmbed{
		Title:       header,
		Description: message,
		Color:       constants.IVY_GREEN, // Green
	}

	// Add footer if provided
	if footer != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: footer,
		}
	}

	// Send the message
	msg, err := s.ChannelMessageSendEmbed(channel.ID, embed)
	if err != nil {
		// User may have blocked the bot
		return nil, err
	}

	return msg, nil
}

// parse an amount string and convert it to IVY
func ParseAmount(amount string) (float64, error) {
	amount = strings.TrimSpace(amount)
	if len(amount) == 0 {
		return 0.0, nil
	}

	var x float64
	if amount[0] == '$' {
		usd, err := strconv.ParseFloat(amount[1:], 64)
		if err != nil {
			return 0.0, err
		}
		x = usd / constants.PRICE.Get(constants.RPC_CLIENT)
	} else {
		var err error
		x, err = strconv.ParseFloat(amount, 64)
		if err != nil {
			return 0.0, err
		}
	}
	if x < 0 {
		return 0.0, errors.New("no negative amounts allowed")
	}
	if x != x {
		return 0.0, errors.New("no NaNs allowed")
	}
	return x, nil
}
