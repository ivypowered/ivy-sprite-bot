package main

import (
	"database/sql"

	"github.com/bwmarrin/discordgo"
)

type CommandFunc func(db *sql.DB, args []string, s *discordgo.Session, m *discordgo.MessageCreate)

type Command struct {
	Name string
	Fn   CommandFunc
}

var commandRegistry = make(map[string]CommandFunc)

func RegisterCommand(name string, fn CommandFunc) {
	commandRegistry[name] = fn
}

func GetCommand(name string) (CommandFunc, bool) {
	fn, exists := commandRegistry[name]
	return fn, exists
}
