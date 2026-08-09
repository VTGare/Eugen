// Package starboard implements the starboard reaction-tracking engine.
package starboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

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

// Event is a queued operation for a single original message.
type Event struct {
	Type      EventType
	ChannelID string // original message channel
	MessageID string // original message id
	GuildID   string
	UserID    string // user who added/removed the reaction

	// For EventMessageDelete: identifies the message that was deleted.
	// Queue resolves it against the starboard records and rewrites
	// ChannelID/MessageID to the original message pair before queueing.
	DeletedChannel  string
	DeletedMessage  string
	DeletedOriginal bool // set by Queue: the deleted message was the original
}

// lane is the single-slot mailbox for one original message.
type lane struct {
	slot      *Event    // newest pending event; nil when idle
	owner     bool      // a worker is currently draining this lane
	tombstone bool      // original message deleted; drop incoming events
	lastUse   time.Time // last enqueue or processing time
}

// Starboarder processes starboard events from Discord reaction and message
// delete events. It serializes operations per original-message so that
// concurrent reaction adds/removes on the same message don't race, while a
// fixed worker pool bounds the number of concurrent Discord API calls.
type Starboarder struct {
	session *discordgo.Session
	store   *store.Store
	log     *slog.Logger
	ctx     context.Context

	// process is the injectable event handler. Queue-level semantics
	// (coalescing, terminal priority) sit in front of it; tests replace it
	// to exercise the pool without a Discord session or database.
	process func(Event) error

	// resolve canonicalizes a message-delete event to its original message
	// pair before queueing. It returns false when the event must be dropped.
	resolve func(*Event) bool

	workers        int
	maxOutstanding int
	ttl            time.Duration

	mu      sync.Mutex
	cond    *sync.Cond
	lanes   map[store.MessagePair]*lane
	fifo    []store.MessagePair // lanes with pending work waiting for a worker
	pending int                 // lanes with work queued or in progress
	stop    chan struct{}
	wg      sync.WaitGroup
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

// WithWorkers bounds the size of the worker pool.
func WithWorkers(n int) Option {
	return func(s *Starboarder) { s.workers = n }
}

// WithMaxOutstanding bounds the number of messages with queued or in-flight
// work. Enqueueing beyond the bound blocks the caller until a worker drains
// a lane.
func WithMaxOutstanding(n int) Option {
	return func(s *Starboarder) { s.maxOutstanding = n }
}

// WithLaneTTL sets how long an idle lane stays alive before eviction.
func WithLaneTTL(ttl time.Duration) Option {
	return func(s *Starboarder) { s.ttl = ttl }
}

func New(ctx context.Context, session *discordgo.Session, st *store.Store, logger *slog.Logger, opts ...Option) *Starboarder {
	if ctx == nil {
		ctx = context.Background()
	}

	sb := &Starboarder{
		session:        session,
		store:          st,
		log:            logger,
		ctx:            ctx,
		workers:        min(80, max(8, runtime.GOMAXPROCS(0)*4)),
		maxOutstanding: 8192,
		ttl:            1 * time.Minute,
		lanes:          make(map[store.MessagePair]*lane),
		stop:           make(chan struct{}),
	}

	sb.cond = sync.NewCond(&sb.mu)
	sb.process = sb.handle
	sb.resolve = sb.resolveDelete

	for _, opt := range opts {
		opt(sb)
	}

	sb.wg.Add(sb.workers)
	for range sb.workers {
		go sb.worker()
	}

	go sb.sweeper()
	go sb.watchContext(ctx)

	return sb
}

// Queue enqueues a starboard event for sequential processing per message.
// It blocks only when the number of messages with pending work is at the
// outstanding cap, which requires a sustained fire across many messages.
func (s *Starboarder) Queue(event Event) {
	if event.Type == EventMessageDelete && !s.resolve(&event) {
		return
	}

	pair := store.NewPair(event.ChannelID, event.MessageID)

	s.mu.Lock()
	defer s.mu.Unlock()

	ln := s.lanes[pair]
	if ln == nil && s.pending >= s.maxOutstanding {
		for s.pending >= s.maxOutstanding {
			s.cond.Wait()
		}
		ln = s.lanes[pair]
	}
	if ln == nil {
		ln = &lane{lastUse: time.Now()}
		s.lanes[pair] = ln
	}

	if ln.tombstone {
		// The original message is gone; only the worker may still process
		// the refills it already owns.
		return
	}

	switch {
	case ln.slot != nil:
		// A newer reaction event supersedes the pending one; a terminal
		// event supersedes everything.
		if isReactionEvent(event.Type) && isReactionEvent(ln.slot.Type) {
			ln.slot = &event
		} else if isTerminalEvent(event.Type) {
			ln.slot = &event
		} else {
			return
		}
	case !ln.owner:
		ln.slot = &event
		s.fifo = append(s.fifo, pair)
		s.pending++
	default:
		// The owning worker's drain loop picks the refill up.
		ln.slot = &event
	}
	ln.lastUse = time.Now()
	s.cond.Signal()
}

// WaitForIdle blocks until every queued or in-flight event has finished
// processing. Useful for graceful shutdown and for tests that must observe
// the pool quiesce before tearing down their dependencies.
func (s *Starboarder) WaitForIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.pending > 0 {
		s.cond.Wait()
	}
}

func (s *Starboarder) worker() {
	defer s.wg.Done()

	for {
		s.mu.Lock()
		for len(s.fifo) == 0 && !s.stopped() {
			s.cond.Wait()
		}
		if s.stopped() && len(s.fifo) == 0 {
			s.mu.Unlock()
			return
		}

		pair := s.fifo[0]
		s.fifo = s.fifo[1:]
		lane := s.lanes[pair]
		event := *lane.slot
		lane.slot = nil
		lane.owner = true
		s.mu.Unlock()

		s.processEvent(pair, event)

		// Drain refills while they keep arriving, so a burst collapses into
		// a single worker cycle processing only the newest state.
		s.mu.Lock()
		for lane.slot != nil {
			if lane.tombstone {
				lane.slot = nil
				break
			}
			next := *lane.slot
			lane.slot = nil
			s.mu.Unlock()
			s.processEvent(pair, next)
			s.mu.Lock()
		}

		lane.owner = false
		lane.lastUse = time.Now()
		s.pending--
		s.cond.Broadcast()
		s.mu.Unlock()
	}
}

func (s *Starboarder) processEvent(pair store.MessagePair, e Event) {
	logger := s.log.With("channel_id", e.ChannelID, "message_id", e.MessageID)
	if err := s.process(e); err != nil {
		logger.Warn("starboard event error", "err", err)
	}

	if e.Type == EventMessageDelete && e.DeletedOriginal {
		s.mu.Lock()
		if lane := s.lanes[pair]; lane != nil {
			lane.tombstone = true
		}
		s.mu.Unlock()
	}
}

func (s *Starboarder) stopped() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
}

// sweeper evicts lanes that have been idle past the TTL so long-lived
// messages don't accumulate queues forever.
func (s *Starboarder) sweeper() {
	ticker := time.NewTicker(max(s.ttl/2, time.Millisecond))
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.sweep()
		}
	}
}

func (s *Starboarder) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for pair, lane := range s.lanes {
		if lane.slot == nil && !lane.owner && now.Sub(lane.lastUse) > s.ttl {
			delete(s.lanes, pair)
		}
	}
}

// watchContext stops the pool when the app context is canceled. Workers
// finish whatever work is already queued before exiting.
func (s *Starboarder) watchContext(ctx context.Context) {
	<-ctx.Done()
	s.mu.Lock()
	close(s.stop)
	s.cond.Broadcast()
	s.mu.Unlock()
	s.wg.Wait()
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
