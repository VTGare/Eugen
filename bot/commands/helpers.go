package commands

import (
	"github.com/VTGare/Eugen/bot/registry"
	"github.com/bwmarrin/discordgo"
)

// cmd is a helper for building registry.Command values during registration.
type cmd struct {
	Name        string
	Description string
	GuildOnly   bool
	Help        *registry.HelpSettings
	Exec        func(*discordgo.Session, *discordgo.MessageCreate, []string) error
}

func (c *cmd) toRegistry() *registry.Command {
	return &registry.Command{
		Name:        c.Name,
		Description: c.Description,
		GuildOnly:   c.GuildOnly,
		Help:        c.Help,
		Exec:        c.Exec,
	}
}

// group creates a new command group.
func group(name, description string, visible bool) *registry.CommandGroup {
	return &registry.CommandGroup{
		Name:        name,
		Description: description,
		IsVisible:   visible,
		Commands:    make(map[string]*registry.Command),
	}
}

// add registers a command into a group.
func add(g *registry.CommandGroup, c *cmd) {
	g.Commands[c.Name] = c.toRegistry()
}
