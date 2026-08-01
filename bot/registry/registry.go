package registry

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Command defines a single bot command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	GuildOnly   bool
	Help        *HelpSettings
	Exec        func(*discordgo.Session, *discordgo.MessageCreate, []string) error
}

type HelpSettings struct {
	IsVisible    bool
	ExtendedHelp []*discordgo.MessageEmbedField
}

type CommandGroup struct {
	Name        string
	Description string
	NSFW        bool
	IsVisible   bool
	Commands    map[string]*Command
}

type Registry struct {
	groups  map[string]*CommandGroup
	aliases map[string]*Command // alias -> command lookup
}

// New creates an empty Registry.
func New() *Registry {
	return &Registry{
		groups:  make(map[string]*CommandGroup),
		aliases: make(map[string]*Command),
	}
}

func (r *Registry) Add(name string, g *CommandGroup) {
	if g.Commands == nil {
		g.Commands = make(map[string]*Command)
	}
	r.groups[name] = g
	for _, cmd := range g.Commands {
		for _, alias := range cmd.Aliases {
			r.aliases[alias] = cmd
		}
	}
}

func (r *Registry) Get(name string) *Command {
	if cmd, ok := r.aliases[name]; ok {
		return cmd
	}
	for _, g := range r.groups {
		if cmd, ok := g.Commands[name]; ok {
			return cmd
		}
	}
	return nil
}

func (r *Registry) Groups() []*CommandGroup {
	out := make([]*CommandGroup, 0, len(r.groups))
	for _, g := range r.groups {
		out = append(out, g)
	}
	return out
}

func (r *Registry) Group(name string) *CommandGroup {
	return r.groups[name]
}

func (c *Command) CreateHelp(prefix string) string {
	str := ""
	if len(c.Aliases) != 0 {
		str += fmt.Sprintf("**Aliases:** %v\n", strings.Join(c.Aliases, ", "))
	}
	str += strings.ReplaceAll(c.Description, "{prefix}", prefix)
	return str
}

func (c *Command) CreateExtendedHelp(prefix string) []*discordgo.MessageEmbedField {
	n := make([]*discordgo.MessageEmbedField, 0)
	if c.Help == nil {
		return n
	}
	for _, h := range c.Help.ExtendedHelp {
		n = append(n, &discordgo.MessageEmbedField{
			Name:   h.Name,
			Value:  strings.ReplaceAll(h.Value, "{prefix}", prefix),
			Inline: false,
		})
	}
	return n
}
