// Package starboard implements the starboard reaction-tracking engine.
package starboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/VTGare/Eugen/bot/starboard/embed"
	"github.com/VTGare/Eugen/store"
	"github.com/bwmarrin/discordgo"
)

// EventType discriminates the kind of starboard operation.
type EventType int

const (
	EventReactionAdd EventType = iota
	EventReactionRemove
	EventMessageDelete
	EventReactionsClear
)

// Event is a lightweight queued operation for a single original message.
// All fields needed for processing are pre-computed by the handler layer,
// so the consumer never blocks on Discord API calls for reaction data.
type Event struct {
	Type      EventType
	ChannelID string // original message channel
	MessageID string // original message id
	GuildID   string
	React     *discordgo.MessageReactions
	Message   *discordgo.Message // fetched by handler when available
	SelfStar  bool               // author added this reaction themselves

	// for EventMessageDelete: identifies the deleted message
	DeletedChannel string
	DeletedMessage string
}

// Starboarder processes starboard events from Discord reaction and message
// delete events. It serializes operations per original-message so that
// concurrent reaction adds/removes on the same message don't race.
type Starboarder struct {
	session *discordgo.Session
	store   *store.Store
	log     *slog.Logger
	ctx     context.Context

	mu     sync.Mutex
	queues map[store.MessagePair]chan Event
}

func New(ctx context.Context, session *discordgo.Session, st *store.Store, logger *slog.Logger) *Starboarder {
	if ctx == nil {
		ctx = context.Background()
	}

	return &Starboarder{
		session: session,
		store:   st,
		log:     logger,
		ctx:     ctx,
		queues:  make(map[store.MessagePair]chan Event),
	}
}

// Queue enqueues a starboard event for sequential processing.
func (s *Starboarder) Queue(event Event) {
	pair := store.NewPair(event.ChannelID, event.MessageID)

	s.mu.Lock()
	ch, ok := s.queues[pair]
	if !ok {
		ch = make(chan Event, 64)
		s.queues[pair] = ch
		go s.consume(pair, ch)
	}
	s.mu.Unlock()

	ch <- event
}

func (s *Starboarder) consume(pair store.MessagePair, ch chan Event) {
	for e := range ch {
		logger := s.log.With("channel_id", pair.ChannelID, "message_id", pair.MessageID)
		if err := s.handle(e, logger); err != nil {
			logger.Warn("starboard event error", "err", err)
		}
	}
}

// ReactionAdd handles a MessageReactionAdd event. The caller is responsible
// for fetching the message and validating guild/emoji/bot checks before calling.
func (s *Starboarder) ReactionAdd(e Event) {
	s.Queue(e)
}

// ReactionRemove handles a MessageReactionRemove event.
func (s *Starboarder) ReactionRemove(e Event) {
	s.Queue(e)
}

// MessageDeleted handles a MessageDelete event.
func (s *Starboarder) MessageDeleted(e Event) {
	s.Queue(e)
}

// ReactionsCleared handles a MessageReactionRemoveAll event.
func (s *Starboarder) ReactionsCleared(e Event) {
	s.Queue(e)
}

func (s *Starboarder) handle(e Event, l *slog.Logger) error {
	switch e.Type {
	case EventReactionAdd:
		return s.handleReactionAdd(s.ctx, e, l)
	case EventReactionRemove:
		return s.handleReactionRemove(s.ctx, e, l)
	case EventMessageDelete:
		return s.handleMessageDelete(s.ctx, e, l)
	case EventReactionsClear:
		return s.handleReactionsClear(s.ctx, e, l)
	default:
		return nil
	}
}

// effectiveCount returns the reaction count after adjusting for self-star.
func effectiveCount(react *discordgo.MessageReactions, selfStar bool, guild *store.Guild) int {
	if react == nil {
		return 0
	}
	count := react.Count
	if selfStar && !guild.Selfstar {
		count--
	}
	return count
}

func (s *Starboarder) handleReactionAdd(ctx context.Context, e Event, l *slog.Logger) error {
	guild := s.store.Guilds.Get(ctx, e.GuildID)
	if guild == nil || !guild.Enabled || guild.StarboardChannel == "" {
		return nil
	}

	react := e.React
	if react == nil || react.Emoji == nil {
		return nil
	}
	if !guild.ValidateEmoji(react.Emoji) {
		return nil
	}

	board, err := s.store.Messages.GetByOriginal(ctx, e.ChannelID, e.MessageID)
	if err != nil {
		return fmt.Errorf("fetching repost: %w", err)
	}

	if board != nil {
		return s.updateStarboardFooter(ctx, board, react, guild, e.SelfStar)
	}

	count := effectiveCount(react, e.SelfStar, guild)
	if count < guild.StarsRequired(e.ChannelID) {
		return nil
	}

	return s.createStarboard(ctx, e, guild, react, l)
}

func (s *Starboarder) handleReactionRemove(ctx context.Context, e Event, l *slog.Logger) error {
	guild := s.store.Guilds.Get(ctx, e.GuildID)
	if guild == nil || !guild.Enabled || guild.StarboardChannel == "" {
		return nil
	}

	react := e.React

	board, err := s.store.Messages.GetByOriginal(ctx, e.ChannelID, e.MessageID)
	if err != nil {
		return fmt.Errorf("fetching repost: %w", err)
	}

	if board == nil {
		return nil
	}

	if react == nil {
		return s.deleteStarboard(ctx, board, l)
	}

	count := effectiveCount(react, e.SelfStar, guild)
	required := guild.StarsRequired(e.ChannelID)

	if count <= required/2 {
		return s.deleteStarboard(ctx, board, l)
	}

	return s.updateStarboardFooter(ctx, board, react, guild, e.SelfStar)
}

func (s *Starboarder) handleMessageDelete(ctx context.Context, e Event, l *slog.Logger) error {
	guild := s.store.Guilds.Get(ctx, e.GuildID)
	if guild == nil || !guild.Enabled || guild.StarboardChannel == "" {
		return nil
	}

	// Could be either the original or the starboard message that was deleted.
	board, err := s.store.Messages.GetByOriginal(ctx, e.DeletedChannel, e.DeletedMessage)
	if err != nil {
		return fmt.Errorf("fetching repost: %w", err)
	}

	if board == nil {
		board, err = s.store.Messages.GetByStarboard(ctx, e.DeletedChannel, e.DeletedMessage)
		if err != nil {
			return fmt.Errorf("fetching repost by starboard: %w", err)
		}
	}

	if board == nil {
		return nil
	}

	return s.deleteStarboardRecord(ctx, board, l)
}

func (s *Starboarder) handleReactionsClear(ctx context.Context, e Event, l *slog.Logger) error {
	guild := s.store.Guilds.Get(ctx, e.GuildID)
	if guild == nil || !guild.Enabled || guild.StarboardChannel == "" {
		return nil
	}

	board, err := s.store.Messages.GetByOriginal(ctx, e.ChannelID, e.MessageID)
	if err != nil {
		return fmt.Errorf("fetching repost: %w", err)
	}

	if board == nil {
		return nil
	}

	return s.deleteStarboard(ctx, board, l)
}

// createStarboard posts a new starboard message and records the pairing.
func (s *Starboarder) createStarboard(ctx context.Context, e Event, guild *store.Guild, react *discordgo.MessageReactions, l *slog.Logger) error {
	original := e.Message
	if original == nil {
		return nil
	}

	ch, err := s.session.Channel(original.ChannelID)
	if err != nil {
		return fmt.Errorf("fetching channel: %w", err)
	}

	builder := embed.NewEmbedBuilder(guild)
	result, err := builder.Build(ch, original, react, effectiveCount(react, e.SelfStar, guild), e.SelfStar)
	if err != nil {
		return fmt.Errorf("building embed: %w", err)
	}
	if result == nil {
		return nil
	}

	l.Debug("creating new starboard")
	starboard, err := s.session.ChannelMessageSendComplex(guild.StarboardChannel, result.Send)
	if err != nil {
		return fmt.Errorf("sending starboard message: %w", err)
	}

	oPair := store.NewPair(original.ChannelID, original.ID)
	sPair := store.NewPair(starboard.ChannelID, starboard.ID)
	if err := s.store.Messages.Create(ctx, store.NewMessage(&oPair, &sPair, guild.ID)); err != nil {
		l.Warn("inserting starboard record", "err", err)
	}

	return nil
}

// updateStarboardFooter edits the footer on an existing starboard message.
func (s *Starboarder) updateStarboardFooter(ctx context.Context, board *store.Message, react *discordgo.MessageReactions, guild *store.Guild, selfStar bool) error {
	msg, err := s.session.ChannelMessage(board.Starboard.ChannelID, board.Starboard.MessageID)
	if err != nil {
		if isNotFound(err) {
			s.log.Info("starboard message not found, removing record")
			return s.store.Messages.Delete(ctx, board.Original)
		}
		return fmt.Errorf("fetching starboard message: %w", err)
	}

	builder := embed.NewEmbedBuilder(guild)
	count := effectiveCount(react, selfStar, guild)
	builder.UpdateFooter(msg.Embeds[0], count, selfStar && guild.Selfstar, react)

	_, err = s.session.ChannelMessageEditEmbed(msg.ChannelID, msg.ID, msg.Embeds[0])
	if err != nil {
		return fmt.Errorf("editing starboard message: %w", err)
	}

	return nil
}

func (s *Starboarder) deleteStarboard(ctx context.Context, board *store.Message, l *slog.Logger) error {
	l.Info("removing starboard", "starboard_id", board.Starboard.MessageID, "channel_id", board.Starboard.ChannelID)
	if err := s.session.ChannelMessageDelete(board.Starboard.ChannelID, board.Starboard.MessageID); err != nil {
		if !isNotFound(err) {
			l.Warn("deleting starboard message", "err", err)
		}
	}

	return s.deleteStarboardRecord(ctx, board, l)
}

func (s *Starboarder) deleteStarboardRecord(ctx context.Context, board *store.Message, l *slog.Logger) error {
	if err := s.store.Messages.Delete(ctx, board.Original); err != nil {
		l.Warn("deleting message record", "err", err)
	}

	pair := *board.Original

	s.mu.Lock()
	if ch, ok := s.queues[pair]; ok {
		close(ch)
		delete(s.queues, pair)
	}
	s.mu.Unlock()

	return nil
}

// isNotFound checks if an error from discordgo is a 404-class error.
var notFoundCodes = map[int]struct{}{
	10003: {}, // unknown channel
	10008: {}, // unknown message
	10014: {}, // unknown emoji
}

func isNotFound(err error) bool {
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) {
		if restErr.Message != nil {
			_, ok := notFoundCodes[restErr.Message.Code]
			return ok
		}
	}

	return false
}
