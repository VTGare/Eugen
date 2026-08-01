package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/VTGare/Eugen/bot/registry"
	"github.com/VTGare/Eugen/store"
	"github.com/VTGare/Eugen/utils"
	"github.com/bwmarrin/discordgo"
)

// Bot is the central application struct. It holds the Discord session,
// the persistence store, the command registry, and the starboard event
// queue. Event handlers and commands are created as closures bound to a
// *Bot instance via constructor functions.
type Bot struct {
	mu         sync.Mutex
	Session    *discordgo.Session
	Store      *store.Store
	Registry   *registry.Registry
	Queue      map[store.MessagePair]chan *StarboardEvent
	Config     Config
	botMention string
	log        *slog.Logger
}

// Config holds bot-level configuration.
type Config struct {
	Prefixes []string
}

// New creates a new Bot with an injected logger.
func New(st *store.Store, config Config, logger *slog.Logger) *Bot {
	if config.Prefixes == nil {
		config.Prefixes = []string{"e!", "e.", "e "}
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Bot{
		Store:    st,
		Config:   config,
		Queue:    make(map[store.MessagePair]chan *StarboardEvent),
		Registry: registry.New(),
		log:      logger,
	}
}

// SetMention sets the bot mention string (called when the session is ready).
func (b *Bot) SetMention(mention string) {
	b.mu.Lock()
	b.botMention = mention
	b.mu.Unlock()
}

// Mention returns the cached bot mention.
func (b *Bot) Mention() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.botMention
}

// Logger returns the bot's logger.
func (b *Bot) Logger() *slog.Logger {
	return b.log
}

// InitGuilds syncs guild data from the ready event into the store.
// It loads all existing guilds into cache, then creates any guilds from
// the Discord ready event that aren't already in the database.
func (b *Bot) InitGuilds(eventGuilds []*discordgo.Guild) {
	ctx := context.Background()
	guilds, err := b.Store.Guilds.All(ctx)
	if err != nil {
		b.log.Warn("loading guilds from db", "err", err)
		return
	}

	cache := b.Store.Guilds.Cache()
	for _, guild := range guilds {
		cache.CacheSet(guild)
	}

	newGuilds := make([]*store.Guild, 0)
	for _, guild := range eventGuilds {
		if cache.Get(guild.ID) == nil {
			b.log.Info("guild not found in db, adding", "guild_id", guild.ID)
			g := store.NewGuild(guild.Name, guild.ID)
			newGuilds = append(newGuilds, g)
			cache.CacheSet(g)
		}
	}

	if len(newGuilds) > 0 {
		if err := b.Store.Guilds.InsertMany(ctx, newGuilds); err != nil {
			b.log.Warn("inserting new guilds", "err", err)
		} else {
			b.log.Info("inserted current guilds", "count", len(newGuilds))
		}
	}

	b.log.Info("connected to guilds", "count", len(eventGuilds))
}

// TrimPrefix removes the command prefix from a message content string.
func (b *Bot) TrimPrefix(content, guildID string) string {
	return b.trimPrefix(content, guildID)
}

func (b *Bot) trimPrefix(content, guildID string) string {
	guild := b.Store.Guilds.Cache().Get(guildID)
	var defaultPrefix bool
	if guild != nil && guild.Prefix == "e!" {
		defaultPrefix = true
	} else if guild == nil {
		defaultPrefix = true
	} else {
		defaultPrefix = false
	}

	switch {
	case startsWithMention(content, b.botMention):
		return trimPrefix(content, b.botMention)
	case defaultPrefix:
		for _, prefix := range b.Config.Prefixes {
			if hasPrefix(content, prefix) {
				return trimPrefix(content, prefix)
			}
		}
	case !defaultPrefix && guild != nil:
		return trimPrefix(content, guild.Prefix)
	default:
		return content
	}

	return content
}

// HandleError logs and optionally notifies about a Discord error.
func (b *Bot) HandleError(s *discordgo.Session, channelID string, err error) {
	if err != nil {
		b.log.Error("handling error", "err", err)
		embed := &discordgo.MessageEmbed{
			Title: "Oops, something went wrong!",
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: "https://i.imgur.com/OZ1Al5h.png",
			},
			Description: fmt.Sprintf("***Error message:***\\n%v\\n", err),
			Color:       utils.EmbedColor,
			Timestamp:   utils.EmbedTimestamp(),
		}
		s.ChannelMessageSendEmbed(channelID, embed)
	}
}

// Push enqueues a starboard event for processing.
func (b *Bot) Push(pair store.MessagePair, event *StarboardEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch, ok := b.Queue[pair]
	if ok {
		ch <- event
	} else {
		ch = make(chan *StarboardEvent)
		b.Queue[pair] = ch
		go b.consume(pair, ch)
		ch <- event
	}
}

// consume processes starboard events from a channel sequentially.
func (b *Bot) consume(pair store.MessagePair, ch chan *StarboardEvent) {
	for e := range ch {
		if err := e.Run(); err != nil {
			b.log.Warn("starboard event error", "err", err)
		}
	}
}

func startsWithMention(content, mention string) bool {
	return len(content) >= len(mention) && content[:len(mention)] == mention
}

func trimPrefix(content, prefix string) string {
	return content[len(prefix):]
}

func hasPrefix(content, prefix string) bool {
	return len(content) >= len(prefix) && content[:len(prefix)] == prefix
}

// FindReact finds a reaction on a message that matches the given emote.
func (b *Bot) FindReact(message *discordgo.Message, emote string) *discordgo.MessageReactions {
	for _, r := range message.Reactions {
		if r.Emoji.MessageFormat() == emote {
			return r
		}
	}
	return nil
}

// NewStarboardEventAdd creates a StarboardEvent for a reaction add.
func (b *Bot) NewStarboardEventAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd, msg *discordgo.Message, react *discordgo.MessageReactions) (*StarboardEvent, error) {
	guild := b.Store.Guilds.Cache().Get(r.GuildID)
	se := &StarboardEvent{
		Guild:       guild,
		Message:     msg,
		Session:     s,
		AddEvent:    r,
		React:       react,
		Store:       b.Store,
		log:         b.log.With("guild_id", guild.ID),
	}
	return se, nil
}

// NewStarboardEventRemove creates a StarboardEvent for a reaction remove.
func (b *Bot) NewStarboardEventRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove, msg *discordgo.Message) (*StarboardEvent, error) {
	guild := b.Store.Guilds.Cache().Get(r.GuildID)
	emote := b.FindReact(msg, guild.StarEmote)
	se := &StarboardEvent{
		Guild:       guild,
		Message:     msg,
		Session:     s,
		RemoveEvent: r,
		React:       emote,
		Store:       b.Store,
		log:         b.log.With("guild_id", guild.ID),
	}
	return se, nil
}

// NewStarboardEventDeleted creates a StarboardEvent for a message delete.
func (b *Bot) NewStarboardEventDeleted(s *discordgo.Session, d *discordgo.MessageDelete) (*StarboardEvent, error) {
	guild := b.Store.Guilds.Cache().Get(d.GuildID)
	return &StarboardEvent{
		Guild:       guild,
		Message:     &discordgo.Message{ID: d.ID, ChannelID: d.ChannelID},
		Session:     s,
		DeleteEvent: d,
		Store:       b.Store,
		log:         b.log.With("guild_id", guild.ID),
	}, nil
}

// RegisterHandler is a convenience for registering a discordgo handler.
func (b *Bot) RegisterHandler(handler any) {
	if b.Session == nil {
		return
	}
	b.Session.AddHandler(handler)
}

// Start creates and assigns the Discord session.
func (b *Bot) Start(token string) error {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("bot: creating session: %w", err)
	}

	s.Identify.Intents = discordgo.IntentsGuildEmojis |
		discordgo.IntentsGuilds |
		discordgo.IntentGuildMessageReactions |
		discordgo.IntentGuildMessages |
		discordgo.IntentMessageContent |
		discordgo.IntentsDirectMessages

	b.Session = s
	return nil
}

// Open opens the Discord connection.
func (b *Bot) Open() error {
	if b.Session == nil {
		return fmt.Errorf("bot: session not created")
	}
	return b.Session.Open()
}

// Close closes the Discord connection.
func (b *Bot) Close() error {
	if b.Session == nil {
		return nil
	}
	return b.Session.Close()
}

// CreateIndexes creates database indexes.
func (b *Bot) CreateIndexes(ctx context.Context) error {
	if err := b.Store.Guilds.CreateIndex(ctx); err != nil {
		return fmt.Errorf("bot: creating guilds index: %w", err)
	}
	if err := b.Store.Messages.CreateIndex(ctx); err != nil {
		return fmt.Errorf("bot: creating messages index: %w", err)
	}
	return nil
}

// LoadGuildCache seeds the guild cache from the database.
func (b *Bot) LoadGuildCache() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if n, err := b.Store.Guilds.LoadIntoCache(ctx); err != nil {
		b.log.Warn("seeding guild cache", "err", err)
	} else {
		b.log.Info("cached guilds", "count", n)
	}
}
