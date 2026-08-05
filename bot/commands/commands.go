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
		Description: "Checks if the bot is online and measures Discord API latency.",
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}ping``", Inline: false},
				{Name: "Description", Value: "Am I alive?", Inline: false},
			},
		},
		Exec: ping(b),
	})

	add(basic, &cmd{
		Name:        "help",
		Description: "Displays a list of all available commands, or detailed help for a specific command.",
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}help`` or ``{prefix}help <command name>``", Inline: false},
				{Name: "Description", Value: "Lists every visible command with a brief description. Supply a command name as an argument to see its detailed documentation, including available settings and permissions.", Inline: false},
			},
		},
		Exec: help(b),
	})

	add(basic, &cmd{
		Name:        "set",
		Description: "Views or modifies the server's configuration.",
		GuildOnly:   true,
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}set`` or ``{prefix}set <setting> <new value>``", Inline: false},
				{Name: "Viewing settings", Value: "Running ``{prefix}set`` with no arguments displays the current guild configuration.", Inline: false},
				{Name: "Available settings", Value: "**prefix** — Changes the bot's command prefix. Maximum 5 characters. If the last character is a letter, a space is appended automatically.\n**enabled** — Enables or disables starboard functionality. Accepts `true`/`false` (case-insensitive).\n**selfstar** — Whether the message author's own reaction counts toward the star threshold. Accepts `true`/`false`.\n**ignorebots** — Whether reactions from bot users should be ignored. Accepts `true`/`false`.\n**starboard** — The channel where starred messages are posted. Accepts a channel ID or mention.\n**emote** — The reaction emote used for starboarding. Must be a guild emoji.\n**stars** — The minimum number of stars required to post a message to the starboard channel. Must be an integer ≥ 1.\n**color** — The embed color (0–16777215, or a hex value like `0xff0000`).", Inline: false},
				{Name: "Examples", Value: "``{prefix}set prefix ?``\n``{prefix}set stars 5``\n``{prefix}set starboard #starboard``\n``{prefix}set color 0xffd700``", Inline: false},
			},
		},
		Exec: set(b),
	})

	add(basic, &cmd{
		Name:        "ban",
		Description: "Prevents one or more channels from being posted to the starboard.",
		GuildOnly:   true,
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}ban <channel mention or ID> [<channel> ...]``", Inline: false},
				{Name: "Description", Value: "Bans the specified channels so that messages posted in them are ignored by the starboard. Multiple channels can be banned at once by separating them with spaces.", Inline: false},
				{Name: "Examples", Value: "``{prefix}ban #general``\n``{prefix}ban #memes #general 123456789012345678``", Inline: false},
			},
		},
		Exec: ban(b),
	})

	add(basic, &cmd{
		Name:        "unban",
		Description: "Allows a previously banned channel to be posted to the starboard again.",
		GuildOnly:   true,
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}unban <channel mention or ID> [<channel> ...]``", Inline: false},
				{Name: "Description", Value: "Removes the ban from one or more channels, allowing them to be posted to the starboard again.", Inline: false},
				{Name: "Examples", Value: "``{prefix}unban #general``\n``{prefix}unban 123456789012345678``", Inline: false},
			},
		},
		Exec: unban(b),
	})

	add(basic, &cmd{
		Name:        "blacklist",
		Description: "Prevents one or more users from appearing on the starboard.",
		GuildOnly:   true,
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}blacklist <user mention or ID> [<user> ...]``", Inline: false},
				{Name: "Description", Value: "Adds one or more users to the blacklist. Blacklisted users' are ignored by the starboard entirely.", Inline: false},
				{Name: "Examples", Value: "``{prefix}blacklist @user``\n``{prefix}blacklist @user1 @user2``", Inline: false},
			},
		},
		Exec: blacklist(b),
	})

	add(basic, &cmd{
		Name:        "unblacklist",
		Description: "Allows a previously blacklisted user's to appear on the starboard.",
		GuildOnly:   true,
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}unblacklist <user mention or ID> [<user> ...]``", Inline: false},
				{Name: "Description", Value: "Removes one or more users from the blacklist.", Inline: false},
				{Name: "Examples", Value: "``{prefix}unblacklist @user``\n``{prefix}unblacklist @user1 @user2``", Inline: false},
			},
		},
		Exec: unblacklist(b),
	})

	add(basic, &cmd{
		Name:        "req",
		Description: "Sets a custom star requirement for specific channels.",
		GuildOnly:   true,
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}req <channel mention or ID> <star requirement>`` or ``{prefix}req <channel mention or ID> default``", Inline: false},
				{Name: "Channel", Value: "Required. The channel to set a custom star requirement for. Must be a channel in this server.", Inline: false},
				{Name: "Star requirement", Value: "Required. Either an integer ≥ 1 (the number of stars needed to post to starboard) or `default` to reset this channel to the server's default star requirement.", Inline: false},
				{Name: "Examples", Value: "``{prefix}req #general 5`` — posts to starboard after 5 stars in #general.\n``{prefix}req #memes default`` — resets #memes to the server default.", Inline: false},
			},
		},
		Exec: req(b),
	})

	add(basic, &cmd{
		Name:        "invite",
		Description: "Sends the bot's invite link so you can add it to your server.",
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}invite``", Inline: false},
				{Name: "Description", Value: "Sends the bot's invite link.", Inline: false},
			},
		},
		Exec: invite(b),
	})

	add(basic, &cmd{
		Name:        "setup",
		Description: "Runs an interactive setup process to configure the starboard.",
		GuildOnly:   true,
		Help: &registry.HelpSettings{
			IsVisible: true,
			ExtendedHelp: []*discordgo.MessageEmbedField{
				{Name: "Usage", Value: "``{prefix}setup``", Inline: false},
				{Name: "Description", Value: "Starts a step-by-step interactive wizard that walks you through configuring the starboard for your server. The steps are: starboard channel, minimum stars, star emote, self-star, and embed color.\n\nAt any step, type `cancel` or `exit` to abort, or `previous` to go back to the prior step.", Inline: false},
				{Name: "Setup steps", Value: "1. **Starboard channel** — mention a channel where starred messages will be posted.\n2. **Minimum stars** — the default number of stars needed to post to the starboard.\n3. **Star emote** — the reaction emote to use (type `default` for ⭐).\n4. **Self-star** — whether the message author's own reaction counts.\n5. **Embed color** — a hex color or integer (0–16777215) for starboard embeds (type `default` for the preset color).", Inline: false},
			},
		},
		Exec: setup(b),
	})

	b.Registry.Add("basic", basic)
}
