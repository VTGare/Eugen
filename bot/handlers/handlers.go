package handlers

import (
	"context"
	"slices"
	"strings"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/bot/registry"
	"github.com/VTGare/Eugen/store"
	"github.com/bwmarrin/discordgo"
)

// Ready returns a handler for the discordgo.Ready event.
func Ready(b *bot.Bot) func(*discordgo.Session, *discordgo.Ready) {
	return func(s *discordgo.Session, e *discordgo.Ready) {
		b.SetMention("<@!" + e.User.ID + ">")
		b.InitGuilds(e.Guilds)
	}
}

// MessageCreate returns a handler for the discordgo.MessageCreate event.
func MessageCreate(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageCreate) {
	return func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}

		isGuild := m.GuildID != ""
		m.Content = strings.ToLower(m.Content)

		content := b.TrimPrefix(m.Content, m.GuildID)
		if content == m.Content {
			return
		}

		fields := strings.Fields(content)
		if len(fields) == 0 {
			return
		}

		for _, group := range b.Registry.Groups() {
			if command, ok := group.Commands[fields[0]]; ok {
				if commandMatch(command, fields[0]) {
					if !isGuild && command.GuildOnly {
						s.ChannelMessageSend(m.ChannelID, "this command can't be executed in DMs or group chats")
						return
					}
					go func(cmd *registry.Command) {
						b.Logger().Info("executing command", "command", m.Content, "user", m.Author.String())
						err := cmd.Exec(s, m, fields[1:])
						b.HandleError(s, m.ChannelID, err)
					}(command)
					break
				}
			}
		}
	}
}

// commandMatch checks if the given name matches the command name or aliases.
func commandMatch(cmd *registry.Command, name string) bool {
	if cmd.Name == name {
		return true
	}
	return slices.Contains(cmd.Aliases, name)
}

// MessageReactionAdd returns a handler for the discordgo.MessageReactionAdd event.
func MessageReactionAdd(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageReactionAdd) {
	return func(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
		guild := b.Store.Guilds.Cache().Get(r.GuildID)
		if guild == nil {
			return
		}
		if !guild.Enabled || guild.StarboardChannel == "" {
			return
		}
		if guild.ValidateEmoji(&r.MessageReaction.Emoji) {
			if guild.IsBanned(r.ChannelID) {
				return
			}
			msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
			if err != nil {
				b.Logger().Warn("fetching message", "err", err, "channel_id", r.ChannelID, "message_id", r.MessageID)
				return
			}
			msg.GuildID = r.GuildID

			if msg.Author != nil {
				if msg.Author.ID == s.State.User.ID {
					return
				}
				if msg.Author.Bot && guild.IgnoreBots {
					return
				}
				if slices.Contains(guild.BlacklistedUsers, msg.Author.ID) {
					return
				}
			}
			if react := b.FindReact(msg, guild.StarEmote); react != nil {
				se, err := b.NewStarboardEventAdd(s, r, msg, react)
				if err != nil {
					b.Logger().Warn("creating starboard event", "err", err)
					return
				}

				if se.React.Count < guild.StarsRequired(se.Message.ChannelID) {
					return
				}
				p := store.NewPair(r.ChannelID, r.MessageID)
				b.Push(p, se)
			}
		}
	}
}

// MessageReactionRemove returns a handler for the discordgo.MessageReactionRemove event.
func MessageReactionRemove(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageReactionRemove) {
	return func(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
		guild := b.Store.Guilds.Cache().Get(r.GuildID)
		if guild == nil {
			return
		}
		if !guild.Enabled || guild.StarboardChannel == "" {
			return
		}
		if guild.ValidateEmoji(&r.MessageReaction.Emoji) {
			if guild.IsBanned(r.ChannelID) {
				return
			}
			msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
			if err != nil {
				b.Logger().Warn("fetching message", "err", err, "channel_id", r.ChannelID, "message_id", r.MessageID)
				return
			}

			if msg.Author != nil {
				if msg.Author.ID == s.State.User.ID {
					return
				}
				if msg.Author.Bot && guild.IgnoreBots {
					return
				}
				if slices.Contains(guild.BlacklistedUsers, msg.Author.ID) {
					return
				}
			}
			se, err := b.NewStarboardEventRemove(s, r, msg)
			if err != nil {
				b.Logger().Warn("creating starboard event", "err", err)
				return
			}
			p := store.NewPair(r.ChannelID, r.MessageID)
			b.Push(p, se)
		}
	}
}

// MessageReactionRemoveAll returns a handler for the discordgo.MessageReactionRemoveAll event.
func MessageReactionRemoveAll(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageReactionRemoveAll) {
	return func(s *discordgo.Session, r *discordgo.MessageReactionRemoveAll) {
		guild := b.Store.Guilds.Cache().Get(r.GuildID)
		if guild == nil {
			return
		}

		msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
		if err != nil {
			b.Logger().Warn("fetching message", "err", err, "channel_id", r.ChannelID, "message_id", r.MessageID)
			return
		}

		if guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(r.ChannelID) && msg.Author.ID != s.State.User.ID {
			repost, err := b.Store.Messages.Repost(context.Background(), r.ChannelID, r.MessageID)
			if err != nil {
				b.Logger().Warn("fetching repost", "err", err)
			}

			if repost != nil {
				b.Logger().Info("removing starboard", "starboard_id", repost.Starboard.MessageID, "channel_id", repost.Starboard.ChannelID, "reason", "all reactions removed")
				err := s.ChannelMessageDelete(repost.Starboard.ChannelID, repost.Starboard.MessageID)
				if err != nil {
					b.Logger().Warn("deleting starboard message", "err", err)
				}
			}
		}
	}
}

// MessageDelete returns a handler for the discordgo.MessageDelete event.
func MessageDelete(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageDelete) {
	return func(s *discordgo.Session, m *discordgo.MessageDelete) {
		guild := b.Store.Guilds.Cache().Get(m.GuildID)
		if guild == nil {
			return
		}

		if guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(m.ChannelID) {
			se, err := b.NewStarboardEventDeleted(s, m)
			if err != nil {
				b.Logger().Warn("creating starboard event", "err", err)
				return
			}
			p := store.NewPair(m.ChannelID, m.ID)
			b.Push(p, se)
		}
	}
}

// GuildCreate returns a handler for the discordgo.GuildCreate event.
func GuildCreate(b *bot.Bot) func(*discordgo.Session, *discordgo.GuildCreate) {
	return func(s *discordgo.Session, g *discordgo.GuildCreate) {
		cache := b.Store.Guilds.Cache()
		if cache.Get(g.ID) != nil {
			return
		}

		newGuild := store.NewGuild(g.Name, g.ID)
		if err := b.Store.Guilds.Insert(context.Background(), newGuild); err != nil {
			b.Logger().Warn("inserting guild", "err", err, "guild_id", g.ID)
		}

		cache.CacheSet(newGuild)
		b.Logger().Info("joined guild", "guild_id", g.ID, "guild_name", g.Name)
	}
}
