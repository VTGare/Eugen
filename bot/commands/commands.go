package commands

import (
	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/bot/registry"
	"github.com/bwmarrin/discordgo"
)

// Register registers all bot commands into the given Bot's registry.
func Register(b *bot.Bot) {
	basic := group("basic", "General purpose commands.", true)

	add(basic, &cmd{
		Name:        "ping",
		Description: "Checks if bot is online and sends a response time.",
		Exec:        ping(b),
	})

	add(basic, &cmd{
		Name:        "help",
		Description: "Sends this message. Use ``{prefix}help <group name> <command name>`` for more info about specific commands. ``{prefix}help <group>`` to list commands in a group.",
		Help: &registry.HelpSettings{
			IsVisible: true,
		},
		Exec: help(b),
	})

	add(basic, &cmd{
		Name:        "set",
		Description: "Show server's settings or change them.",
		Aliases:     []string{"settings", "config", "cfg"},
		GuildOnly:   true,
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "{prefix}set ``<setting>`` ``<new setting>``"},
				{Name: "prefix", Value: "Changes bot's prefix. Maximum ***5 characters***. If last character is a letter whitespace is assumed (takes one character)."},
				{Name: "enabled", Value: "Starboard functionality switch, accepts ***f or false (case-insensitive)*** to disable and ***t or true*** to enable."},
				{Name: "starboard", Value: "Starboard channel. Required for starboard to work. Accepts channel ID or channel mention."},
				{Name: "emote", Value: "Starboard reaction emote."},
				{Name: "stars", Value: "Stars required to repost a message to starboard channel."},
			},
		},
		Exec: set(b),
	})

	add(basic, &cmd{
		Name:        "ban",
		Description: "Bans a channel",
		GuildOnly:   true,
		Exec:        ban(b),
	})

	add(basic, &cmd{
		Name:        "unban",
		Description: "Unbans a channel",
		GuildOnly:   true,
		Exec:        unban(b),
	})

	add(basic, &cmd{
		Name:        "blacklist",
		Description: "Blacklists a user",
		GuildOnly:   true,
		Exec:        blacklist(b),
	})

	add(basic, &cmd{
		Name:        "unblacklist",
		Description: "Unblacklists a user",
		GuildOnly:   true,
		Exec:        unblacklist(b),
	})

	add(basic, &cmd{
		Name:        "req",
		Description: "Sets per channel star requirement",
		GuildOnly:   true,
		Aliases:     []string{"requirement", "channelstars", "channelset"},
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "e!req <channel id or mention> <star requirement>"},
				{Name: "Channel ID or mention", Value: "Required. It must be a channel on this server!"},
				{Name: "Star requirement", Value: "Required. It must be an integer greater than or equal to 1 or ``default`` to remove a custom star requirement."},
			},
		},
		Exec: req(b),
	})

	add(basic, &cmd{
		Name:        "invite",
		Description: "Sends an invite link",
		Exec:        invite(b),
	})

	add(basic, &cmd{
		Name:        "setup",
		Description: "Starts an interactive Eugen setup process.",
		GuildOnly:   true,
		Exec:        setup(b),
	})

	b.Registry.Add("basic", basic)
}
