package commands

import "github.com/VTGare/Eugen/bot"

func RegisterCommands(b *bot.Bot) {
	b.AddCommand(ping)
	b.AddCommand(ignore)
}
