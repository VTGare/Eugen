package commands

import (
	"fmt"
	"time"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/utils"
	"github.com/bwmarrin/discordgo"
)

func ping(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		embed := utils.BaseEmbed(s)
		embed.Title = "🏓 Pong!"
		embed.Fields = []*discordgo.MessageEmbedField{{
			Name:   "Heartbeat latency",
			Value:  fmt.Sprintf("%v", s.HeartbeatLatency().Round(1 * time.Millisecond)),
			Inline: true,
		}}

		_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return err
	}
}

func help(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate, []string) error {
	return func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
		guild := b.Store.Guilds.Cache().Get(m.GuildID)
		var prefix string
		if guild != nil {
			prefix = guild.Prefix
		} else {
			prefix = "e!"
		}

		embed := &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Use ``%vhelp <command name>`` for extended help on specific commands.", prefix),
			Color:       utils.EmbedColor,
			Timestamp:   utils.EmbedTimestamp(),
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: "https://i.imgur.com/OZ1Al5h.png",
			},
		}

		switch len(args) {
		case 0:
			embed.Title = "Help"
			for _, group := range b.Registry.Groups() {
				if group.IsVisible {
					seen := make(map[string]bool)
					for _, command := range group.Commands {
						if !seen[command.Name] {
							embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
								Name:   command.Name,
								Value:  command.CreateHelp(prefix),
								Inline: false,
							})
							seen[command.Name] = true
						}
					}
				}
			}
		case 1:
			found := false
			for _, group := range b.Registry.Groups() {
				if command, ok := group.Commands[args[0]]; ok {
					if len(command.CreateExtendedHelp(prefix)) > 0 && command.Help.IsVisible {
						found = true
						embed.Title = fmt.Sprintf("%v command extended help", command.Name)
						embed.Fields = command.CreateExtendedHelp(prefix)
					}
				}
			}
			if !found {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Command %v either doesn't have extended help info or doesn't exist.", args[0]))
				return nil
			}
		default:
			return fmt.Errorf("incorrect command usage. Example: e!help <command name>")
		}

		s.ChannelMessageSendEmbed(m.ChannelID, embed)
		return nil
	}
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
