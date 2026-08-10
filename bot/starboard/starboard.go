// Package starboard implements the starboard reaction-tracking engine.
package starboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
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

func (t EventType) String() string {
	switch t {
	case EventReactionAdd:
		return "reaction_add"
	case EventReactionRemove:
		return "reaction_remove"
	case EventMessageDelete:
		return "message_delete"
	case EventReactionsClear:
		return "reactions_clear"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// Event is a queued operation for a single original message.
type Event struct {
	Type      EventType
	ChannelID string // original message channel
	MessageID string // original message id
	GuildID   string
	UserID    string // user who added/removed the reaction

	// For EventMessageDelete: identifies the message that was deleted.
	DeletedChannel  string
	DeletedMessage  string
	DeletedOriginal bool // the deleted message was the original
}

// actor owns the processing of one original message. It runs as its own
// goroutine while the message has queued work and returns as soon as it goes
// idle.
//
// A single actor is the serialization point for its message: pending events
// are coalesced under the Starboarder mutex, so a burst of reactions for the
// same message collapses into the newest state before the actor sees it.
type actor struct {
	sb      *Starboarder
	key     store.MessagePair
	pending *Event // newest coalesced event; guarded by sb.mu
}

// Starboarder processes starboard events. It guarantees per-message
// sequential processing while bounding how many messages are handled
// concurrently:
//   - One actor goroutine per active original message, holding at most one
//     pending event. A burst collapses to the newest state because the
//     reaction count is always re-fetched at processing time.
//   - A semaphore of maxActors slots bounds the number of live actors, which
//     also bounds concurrent Discord API calls. Enqueueing work for a new
//     message beyond the cap blocks the caller until an actor goes idle and
//     frees a slot.
type Starboarder struct {
	session *discordgo.Session
	store   *store.Store
	log     *slog.Logger
	ctx     context.Context

	process func(Event) error
	resolve func(*Event) bool

	maxActors int

	mu     sync.Mutex
	actors map[store.MessagePair]*actor
	slots  chan struct{}  // capacity maxActors; one token per live actor
	wg     sync.WaitGroup // counts in-flight events for WaitForIdle
}

type Option func(*Starboarder)

func WithProcess(process func(Event) error) Option {
	return func(s *Starboarder) { s.process = process }
}

// WithResolve overrides how message-delete events are canonicalized to their
// original message pair. The default resolves against the starboard records.
func WithResolve(resolve func(*Event) bool) Option {
	return func(s *Starboarder) { s.resolve = resolve }
}

// WithMaxActors bounds how many messages may be processed concurrently. The
// same bound caps live actor goroutines and, as a side effect, concurrent
// Discord API calls.
func WithMaxActors(n int) Option {
	return func(s *Starboarder) { s.maxActors = n }
}

func New(ctx context.Context, session *discordgo.Session, st *store.Store, logger *slog.Logger, opts ...Option) *Starboarder {
	if ctx == nil {
		ctx = context.Background()
	}

	sb := &Starboarder{
		session:   session,
		store:     st,
		log:       logger,
		ctx:       ctx,
		maxActors: 512,
		actors:    make(map[store.MessagePair]*actor),
	}

	sb.process = sb.handle
	sb.resolve = sb.resolveDelete

	for _, opt := range opts {
		opt(sb)
	}

	if sb.maxActors < 1 {
		sb.maxActors = 1
	}
	sb.slots = make(chan struct{}, sb.maxActors)

	return sb
}

// Queue enqueues an event for per-message sequential processing. It blocks
// only when maxActors messages are already being processed concurrently,
// which requires sustained pressure across many messages at once.
func (s *Starboarder) Queue(event Event) {
	if event.Type == EventMessageDelete && !s.resolve(&event) {
		return
	}

	if s.ctx.Err() != nil {
		return
	}

	pair := store.NewPair(event.ChannelID, event.MessageID)

	s.mu.Lock()
	a := s.actors[pair]
	if a == nil {
		// No actor yet: take a slot, then re-check under the lock in case
		// another goroutine created the actor while we waited for capacity.
		s.mu.Unlock()
		select {
		case s.slots <- struct{}{}:
		case <-s.ctx.Done():
			return
		}
		s.mu.Lock()
		if a = s.actors[pair]; a == nil {
			a = &actor{sb: s, key: pair}
			s.actors[pair] = a
			go a.run()
		} else {
			<-s.slots
		}
	}

	if a.pending != nil {
		switch {
		case isReactionEvent(event.Type) && isReactionEvent(a.pending.Type):
			a.pending = &event
			s.log.Debug(
				"replaced pending reaction",
				"type", event.Type.String(),
				"channel_id", event.ChannelID,
				"message_id", event.MessageID,
				"user_id", event.UserID,
			)

		case isTerminalEvent(event.Type):
			a.pending = &event
			s.log.Debug(
				"superseded pending event with terminal",
				"type", event.Type.String(),
				"channel_id", event.ChannelID,
				"message_id", event.MessageID,
			)

		default:
			// A reaction behind a pending terminal event is dropped: the FSM
			// converges on the newest reaction state at processing time anyway.
			s.log.Debug(
				"dropped reaction behind pending terminal",
				"type", event.Type.String(),
				"channel_id", event.ChannelID,
				"message_id", event.MessageID,
				"user_id", event.UserID,
			)
		}
	} else {
		a.pending = &event
		s.wg.Add(1)
		s.log.Debug(
			"queued new event",
			"type", event.Type.String(),
			"channel_id", event.ChannelID,
			"message_id", event.MessageID,
			"user_id", event.UserID,
		)
	}
	s.mu.Unlock()
}

// WaitForIdle blocks until every queued or in-flight event has finished
// processing. Useful for graceful shutdown and for tests that must observe
// the pool quiesce before tearing down their dependencies.
func (s *Starboarder) WaitForIdle() {
	s.wg.Wait()
}

// run processes the actor's events in order until it goes idle, then frees
// its slot and removes itself.
func (a *actor) run() {
	for {
		a.sb.mu.Lock()
		if a.pending == nil {
			delete(a.sb.actors, a.key)
			a.sb.mu.Unlock()
			<-a.sb.slots
			return
		}

		ev := a.pending
		a.pending = nil
		a.sb.mu.Unlock()

		a.sb.log.Debug("processing event",
			"type", ev.Type.String(),
			"channel_id", ev.ChannelID,
			"message_id", ev.MessageID,
			"user_id", ev.UserID)

		if err := a.sb.process(*ev); err != nil {
			a.sb.log.With("channel_id", ev.ChannelID, "message_id", ev.MessageID).
				Warn("starboard event error", "err", err)
		}
		a.sb.wg.Done()
	}
}

// resolveDelete resolves a message-delete event to the original message
// pair that owns the starboard record, so delete processing serializes
// against the original's reaction events. Returns false when the deleted
// message is not starboarded.
func (s *Starboarder) resolveDelete(event *Event) bool {
	board, err := s.store.Messages.GetByOriginal(s.ctx, event.DeletedChannel, event.DeletedMessage)
	if err != nil {
		s.log.Warn("resolving deleted message", "err", err, "channel_id", event.DeletedChannel, "message_id", event.DeletedMessage)
		return false
	}

	if board != nil {
		event.DeletedOriginal = true
	} else {
		board, err = s.store.Messages.GetByStarboard(s.ctx, event.DeletedChannel, event.DeletedMessage)
		if err != nil {
			s.log.Warn("resolving deleted starboard", "err", err, "channel_id", event.DeletedChannel, "message_id", event.DeletedMessage)
			return false
		}

		if board == nil {
			return false
		}
	}

	event.ChannelID = board.Original.ChannelID
	event.MessageID = board.Original.MessageID
	return true
}

// ReactionAdd handles a MessageReactionAdd event.
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

func isReactionEvent(t EventType) bool {
	return t == EventReactionAdd || t == EventReactionRemove
}

func isTerminalEvent(t EventType) bool {
	return t == EventMessageDelete || t == EventReactionsClear
}

// handle dispatches a queued event. Reaction add/remove converge on a single
// state machine driven by the current reaction count and record existence,
// so dropped intermediate events never change the outcome.
func (s *Starboarder) handle(e Event) error {
	switch e.Type {
	case EventReactionAdd, EventReactionRemove:
		return s.handleReaction(s.ctx, e)
	case EventMessageDelete:
		return s.handleMessageDelete(s.ctx, e)
	case EventReactionsClear:
		return s.handleReactionsClear(s.ctx, e)
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

// findReaction finds a reaction on a message that matches the given emote.
func findReaction(message *discordgo.Message, emote string) *discordgo.MessageReactions {
	for _, r := range message.Reactions {
		if strings.EqualFold(r.Emoji.MessageFormat(), emote) {
			return r
		}
	}
	return nil
}

// handleReaction processes a reaction add or remove event. The original
// message is fetched lazily at processing time, so the reaction count always
// reflects Discord's newest state.
func (s *Starboarder) handleReaction(ctx context.Context, e Event) error {
	guild := s.store.Guilds.Get(ctx, e.GuildID)
	if guild == nil || !guild.Enabled || guild.StarboardChannel == "" {
		return nil
	}

	msg, err := s.session.ChannelMessage(e.ChannelID, e.MessageID)
	if err != nil {
		s.log.Warn("fetching message", "err", err, "channel_id", e.ChannelID, "message_id", e.MessageID)
		return nil
	}
	msg.GuildID = e.GuildID

	if msg.Author == nil {
		return nil
	}

	if msg.Author.ID == s.session.State.User.ID {
		return nil
	}

	if msg.Author.Bot && guild.IgnoreBots {
		return nil
	}

	if slices.Contains(guild.BlacklistedUsers, msg.Author.ID) {
		return nil
	}

	react := findReaction(msg, guild.StarEmote)
	selfStar := e.UserID != "" && msg.Author.ID == e.UserID
	count := effectiveCount(react, selfStar, guild)
	required := guild.StarsRequired(e.ChannelID)

	board, err := s.store.Messages.GetByOriginal(ctx, e.ChannelID, e.MessageID)
	if err != nil {
		return fmt.Errorf("fetching repost: %w", err)
	}

	if board == nil {
		if count >= required {
			return s.createStarboard(ctx, msg, react, count, selfStar, guild)
		}

		return nil
	}

	// The post lives in a channel that no longer is the starboard channel.
	// Relocate the post by deleting the old one and reposting,
	// or delete it entirely when the reaction count no longer qualifies.
	moved := board.Starboard.ChannelID != guild.StarboardChannel
	if moved && count >= required {
		err := s.session.ChannelMessageDelete(board.Starboard.ChannelID, board.Starboard.MessageID)
		if err != nil && !isNotFound(err) {
			return fmt.Errorf("deleting old starboard post: %w", err)
		}

		if err := s.store.Messages.Delete(ctx, board.Original); err != nil {
			return fmt.Errorf("deleting old starboard record: %w", err)
		}

		s.log.Info("moving starboard to new channel",
			"old_channel_id", board.Starboard.ChannelID,
			"new_channel_id", guild.StarboardChannel)
		return s.createStarboard(ctx, msg, react, count, selfStar, guild)
	}

	if moved || count <= required/2 {
		return s.deleteStarboard(ctx, board, s.log)
	}

	return s.updateStarboardFooter(ctx, board, react, guild, selfStar)
}

func (s *Starboarder) handleMessageDelete(ctx context.Context, e Event) error {
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

	if e.DeletedOriginal {
		// The original message is gone, the starboard post must go too.
		return s.deleteStarboard(ctx, board, s.log)
	}

	// The deleted message was the starboard post itself, only the record
	// needs cleanup. A move may have replaced the record's post since the
	// event was routed, only touch the record when it still points at the
	// deleted message.
	if board.Starboard == nil ||
		board.Starboard.ChannelID != e.DeletedChannel ||
		board.Starboard.MessageID != e.DeletedMessage {
		return nil
	}

	return s.deleteStarboardRecord(ctx, board, s.log)
}

func (s *Starboarder) handleReactionsClear(ctx context.Context, e Event) error {
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

	msg, err := s.session.ChannelMessage(e.ChannelID, e.MessageID)
	if err != nil {
		if isNotFound(err) {
			return nil
		}

		return fmt.Errorf("fetching message: %w", err)
	}

	// Never remove a starboard when the reactions were cleared on the
	// starboard post itself (the bot owns it).
	if msg.Author != nil && msg.Author.ID == s.session.State.User.ID {
		return nil
	}

	return s.deleteStarboard(ctx, board, s.log)
}

// createStarboard posts a new starboard message and records the pairing.
func (s *Starboarder) createStarboard(ctx context.Context, original *discordgo.Message, react *discordgo.MessageReactions, count int, selfStar bool, guild *store.Guild) error {
	ch, err := s.session.Channel(original.ChannelID)
	if err != nil {
		return fmt.Errorf("fetching channel: %w", err)
	}

	builder := embed.NewEmbedBuilder(guild)
	result, err := builder.Build(ch, original, react, count, selfStar)
	if err != nil {
		return fmt.Errorf("building embed: %w", err)
	}
	if result == nil {
		return nil
	}

	s.log.Debug("creating new starboard", "channel_id", original.ChannelID, "message_id", original.ID)
	starboard, err := s.session.ChannelMessageSendComplex(guild.StarboardChannel, result.Send)
	if err != nil {
		return fmt.Errorf("sending starboard message: %w", err)
	}

	oPair := store.NewPair(original.ChannelID, original.ID)
	sPair := store.NewPair(starboard.ChannelID, starboard.ID)
	if err := s.store.Messages.Create(ctx, store.NewMessage(&oPair, &sPair, guild.ID)); err != nil {
		s.log.Warn("inserting starboard record", "err", err)
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
