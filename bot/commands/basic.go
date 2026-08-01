package commands

import (
	"fmt"
	"sort"
	"time"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/bot/registry"
	"github.com/VTGare/Eugen/utils"
	"github.com/VTGare/embeds"
	"github.com/bwmarrin/discordgo"
)

func ping(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		embed := utils.BaseEmbed(s)
		embed.Title = "🏓 Pong!"
		embed.Fields = []*discordgo.MessageEmbedField{{
			Name:   "Heartbeat latency",
			Value:  fmt.Sprintf("%v", s.HeartbeatLatency().Round(1*time.Millisecond)),
			Inline: true,
		}}

		_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return err
	}
}

func help(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		prefix := defaultPrefix(b, m.GuildID)

		switch len(args) {
		case 0:
			return sendCommandList(s, m, b, prefix)
		case 1:
			return sendCommandHelp(s, m, b, prefix, args[0])
		default:
			return fmt.Errorf("incorrect command usage. Example: ``%vhelp <command name>``", prefix)
		}
	}
}

// defaultPrefix returns the guild's custom prefix if set, otherwise "e!".
func defaultPrefix(b *bot.Bot, guildID string) string {
	guild := b.Store.Guilds.Cache().Get(guildID)
	if guild != nil && guild.Prefix != "" {
		return guild.Prefix
	}
	return "e!"
}

// sendCommandList builds a flattened embed listing every visible command
// across all groups. Group headers are intentionally omitted: the help command
// only resolves command names, never group names. Commands are sorted
// alphabetically by name for deterministic output.
func sendCommandList(s *discordgo.Session, m *discordgo.MessageCreate, b *bot.Bot, prefix string) error {
	eb := embeds.NewBuilder().
		Title("Commands").
		Description(fmt.Sprintf("Use ``%vhelp <command name>`` for extended help on a specific command.", prefix)).
		Color(utils.EmbedColor).
		Thumbnail(s.State.User.AvatarURL("")).
		Timestamp(time.Now())

	// Collect deduplicated commands, then sort alphabetically for stable output.
	seen := make(map[string]bool)
	commands := make([]*registry.Command, 0)
	for _, group := range b.Registry.Groups() {
		if !group.IsVisible {
			continue
		}
		for _, command := range group.Commands {
			if seen[command.Name] {
				continue
			}
			seen[command.Name] = true
			commands = append(commands, command)
		}
	}

	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	for _, command := range commands {
		eb.AddField(command.Name, command.CreateHelp(prefix), true)
	}

	_, err := s.ChannelMessageSendEmbed(m.ChannelID, eb.Finalize())
	return err
}

// sendCommandHelp resolves a single command by name and sends its
// extended help embed.
func sendCommandHelp(s *discordgo.Session, m *discordgo.MessageCreate, b *bot.Bot, prefix, name string) error {
	command := b.Registry.Get(name)
	if command == nil {
		eb := embeds.NewBuilder().
			ErrorTemplate(fmt.Sprintf("Unknown command: ``%v``.", name)).
			Timestamp(time.Now())
		_, err := s.ChannelMessageSendEmbed(m.ChannelID, eb.Finalize())
		return err
	}

	extended := command.CreateExtendedHelp(prefix)
	if len(extended) == 0 {
		eb := embeds.NewBuilder().
			ErrorTemplate(fmt.Sprintf("Command ``%v`` has no extended help available.", command.Name)).
			Timestamp(time.Now())
		_, err := s.ChannelMessageSendEmbed(m.ChannelID, eb.Finalize())
		return err
	}

	eb := embeds.NewBuilder().
		Title(fmt.Sprintf("%v - help", command.Name)).
		Color(utils.EmbedColor).
		Thumbnail(s.State.User.AvatarURL("")).
		Timestamp(time.Now())

	for _, field := range extended {
		eb.AddField(field.Name, field.Value, false)
	}

	_, err := s.ChannelMessageSendEmbed(m.ChannelID, eb.Finalize())
	return err
}

func invite(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		embed := &discordgo.MessageEmbed{
			Title:       "Thanks for spreading the word!",
			Description: "Eugen loves you 💖\nhttps://discord.com/api/oauth2/authorize?client_id=738399095378673786&permissions=379968&scope=bot",
			Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: s.State.User.AvatarURL("")},
			Color:       utils.EmbedColor,
			Timestamp:   utils.EmbedTimestamp(),
		}
		s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return nil
	}
}
