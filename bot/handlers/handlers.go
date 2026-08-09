package handlers

import (
	"strings"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/bot/registry"
	"github.com/VTGare/Eugen/bot/starboard"
	"github.com/VTGare/Eugen/store"
	"github.com/bwmarrin/discordgo"
)

// Ready returns a handler for the discordgo.Ready event.
func Ready(b *bot.Bot) func(*discordgo.Session, *discordgo.Ready) {
	return func(s *discordgo.Session, e *discordgo.Ready) {
		b.SetMention("<@" + e.User.ID + ">")
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

// commandMatch checks if the given name matches the command name.
func commandMatch(cmd *registry.Command, name string) bool {
	return cmd.Name == name
}

// MessageReactionAdd returns a handler for the discordgo.MessageReactionAdd event.
func MessageReactionAdd(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageReactionAdd) {
	return func(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
		guild := b.Store.Guilds.Get(b.Context(), r.GuildID)
		if guild == nil || !guild.Enabled || guild.StarboardChannel == "" {
			return
		}

		if !guild.ValidateEmoji(&r.MessageReaction.Emoji) {
			return
		}

		if guild.IsBanned(r.ChannelID) {
			return
		}

		b.Starboard.ReactionAdd(starboard.Event{
			Type:      starboard.EventReactionAdd,
			ChannelID: r.ChannelID,
			MessageID: r.MessageID,
			GuildID:   r.GuildID,
			UserID:    r.UserID,
		})
	}
}

// MessageReactionRemove returns a handler for the discordgo.MessageReactionRemove event.
func MessageReactionRemove(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageReactionRemove) {
	return func(s *discordgo.Session, r *discordgo.MessageReactionRemove) {
		guild := b.Store.Guilds.Get(b.Context(), r.GuildID)
		if guild == nil || !guild.Enabled || guild.StarboardChannel == "" {
			return
		}

		if !guild.ValidateEmoji(&r.MessageReaction.Emoji) {
			return
		}

		if guild.IsBanned(r.ChannelID) {
			return
		}

		b.Starboard.ReactionRemove(starboard.Event{
			Type:      starboard.EventReactionRemove,
			ChannelID: r.ChannelID,
			MessageID: r.MessageID,
			GuildID:   r.GuildID,
			UserID:    r.UserID,
		})
	}
}

// MessageReactionRemoveAll returns a handler for the discordgo.MessageReactionRemoveAll event.
func MessageReactionRemoveAll(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageReactionRemoveAll) {
	return func(s *discordgo.Session, r *discordgo.MessageReactionRemoveAll) {
		guild := b.Store.Guilds.Get(b.Context(), r.GuildID)
		if guild == nil || !guild.Enabled || guild.StarboardChannel == "" {
			return
		}

		if guild.IsBanned(r.ChannelID) {
			return
		}

		b.Starboard.ReactionsCleared(starboard.Event{
			Type:      starboard.EventReactionsClear,
			ChannelID: r.ChannelID,
			MessageID: r.MessageID,
			GuildID:   r.GuildID,
		})
	}
}

// MessageDelete returns a handler for the discordgo.MessageDelete event.
func MessageDelete(b *bot.Bot) func(*discordgo.Session, *discordgo.MessageDelete) {
	return func(s *discordgo.Session, m *discordgo.MessageDelete) {
		guild := b.Store.Guilds.Get(b.Context(), m.GuildID)
		if guild == nil {
			return
		}
		if guild.Enabled && guild.StarboardChannel != "" && !guild.IsBanned(m.ChannelID) {
			b.Starboard.MessageDeleted(starboard.Event{
				Type:           starboard.EventMessageDelete,
				ChannelID:      m.ChannelID,
				MessageID:      m.ID,
				GuildID:        m.GuildID,
				DeletedChannel: m.ChannelID,
				DeletedMessage: m.ID,
			})
		}
	}
}

// GuildCreate returns a handler for the discordgo.GuildCreate event.
func GuildCreate(b *bot.Bot) func(*discordgo.Session, *discordgo.GuildCreate) {
	return func(s *discordgo.Session, g *discordgo.GuildCreate) {
		if b.Store.Guilds.Get(b.Context(), g.ID) != nil {
			return
		}

		newGuild := store.NewGuild(g.Name, g.ID)
		if err := b.Store.Guilds.Create(b.Context(), newGuild); err != nil {
			b.Logger().Warn("inserting guild", "err", err, "guild_id", g.ID)
		}

		b.Logger().Info("joined guild", "guild_id", g.ID, "guild_name", g.Name)
	}
}
