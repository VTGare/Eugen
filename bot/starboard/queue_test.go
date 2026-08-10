package starboard_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/Eugen/bot/starboard"
)

// recorder captures processed events in order.
type recorder struct {
	mu     sync.Mutex
	events []starboard.Event
}

func (r *recorder) add(e starboard.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) all() []starboard.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]starboard.Event(nil), r.events...)
}

func (r *recorder) types() []starboard.EventType {
	out := make([]starboard.EventType, 0)
	for _, e := range r.all() {
		out = append(out, e.Type)
	}
	return out
}

func (r *recorder) containsType(t starboard.EventType) bool {
	for _, got := range r.all() {
		if got.Type == t {
			return true
		}
	}
	return false
}

func newRecorder() *recorder { return &recorder{} }

// gatedProcess blocks the first invocation until door is closed (or proceeds
// immediately when door is nil) and signals started once the first event is
// being processed. All events are recorded.
func gatedProcess(rec *recorder, started, door chan struct{}) func(starboard.Event) error {
	var mu sync.Mutex
	first := true
	return func(e starboard.Event) error {
		mu.Lock()
		if first {
			first = false
			mu.Unlock()
			if started != nil {
				close(started)
			}
			if door != nil {
				<-door
			}
		} else {
			mu.Unlock()
		}
		rec.add(e)
		return nil
	}
}

func passResolve(e *starboard.Event) bool { return true }

func newQueueStarboarder(maxActors int, process func(starboard.Event) error, resolve func(*starboard.Event) bool) *starboard.Starboarder {
	if process == nil {
		panic("process must be set")
	}
	if resolve == nil {
		resolve = passResolve
	}
	log := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return starboard.New(
		context.Background(), nil, nil, log,
		starboard.WithProcess(process),
		starboard.WithResolve(resolve),
		starboard.WithMaxActors(maxActors),
	)
}

func queueAddEvent(messageID, userID string) starboard.Event {
	return starboard.Event{
		Type:      starboard.EventReactionAdd,
		ChannelID: "chan1",
		MessageID: messageID,
		GuildID:   "guild1",
		UserID:    userID,
	}
}

func queueDeleteEvent(messageID string) starboard.Event {
	return starboard.Event{
		Type:           starboard.EventMessageDelete,
		ChannelID:      "chan1",
		MessageID:      messageID,
		GuildID:        "guild1",
		DeletedChannel: "chan1",
		DeletedMessage: messageID,
	}
}

var _ = Describe("Starboarder queue mechanics", func() {
	It("collapses a burst of reaction events into the newest state", func() {
		rec := newRecorder()
		started := make(chan struct{})
		door := make(chan struct{})
		sb := newQueueStarboarder(1, gatedProcess(rec, started, door), nil)

		sb.Queue(queueAddEvent("msg1", "u1"))
		<-started

		for i := 2; i <= 5; i++ {
			sb.Queue(queueAddEvent("msg1", fmt.Sprintf("u%d", i)))
		}
		close(door)

		Eventually(rec.types).Should(Equal([]starboard.EventType{
			starboard.EventReactionAdd,
			starboard.EventReactionAdd,
		}))
		Eventually(func() []string {
			users := make([]string, 0)
			for _, e := range rec.all() {
				users = append(users, e.UserID)
			}
			return users
		}).Should(Equal([]string{"u1", "u5"}))
	})

	It("processes queued events in order, always ending with the newest", func() {
		rec := newRecorder()
		sb := newQueueStarboarder(1, gatedProcess(rec, nil, nil), nil)

		for i := 1; i <= 10; i++ {
			sb.Queue(queueAddEvent(fmt.Sprintf("msg%d", i), ""))
		}

		sb.WaitForIdle()
		events := rec.all()
		Expect(events).To(HaveLen(10), "distinct messages must all be processed")
		Expect(events[len(events)-1].MessageID).To(Equal("msg10"),
			"the newest enqueued event must always be processed")
	})

	It("never lets a reaction event supersede a pending terminal event", func() {
		rec := newRecorder()
		started := make(chan struct{})
		door := make(chan struct{})
		sb := newQueueStarboarder(1, gatedProcess(rec, started, door), nil)

		sb.Queue(queueAddEvent("msg1", "u1"))
		<-started

		sb.Queue(queueAddEvent("msg1", "u2"))
		sb.Queue(queueDeleteEvent("msg1"))
		sb.Queue(queueAddEvent("msg1", "u3")) // dropped: reaction behind terminal

		close(door)

		Eventually(rec.types).Should(Equal([]starboard.EventType{
			starboard.EventReactionAdd,
			starboard.EventMessageDelete,
		}))
	})

	It("blocks enqueue beyond the actor cap until an actor goes idle", func() {
		rec := newRecorder()
		started := make(chan struct{})
		door := make(chan struct{})
		sb := newQueueStarboarder(1, gatedProcess(rec, started, door), nil)

		sb.Queue(queueAddEvent("msg1", "u1"))
		<-started

		queued := make(chan struct{})
		go func() {
			defer close(queued)
			sb.Queue(queueAddEvent("msg2", "u2"))
		}()

		Consistently(queued, 50*time.Millisecond).ShouldNot(BeClosed(),
			"enqueue must block while the actor cap is reached")

		close(door)
		Eventually(queued).Should(BeClosed())
		Eventually(rec.types).Should(ContainElement(starboard.EventReactionAdd))
	})

	It("handles concurrent enqueues without deadlock and preserves per-message order", func() {
		rec := newRecorder()
		sb := newQueueStarboarder(4, gatedProcess(rec, nil, nil), nil)

		const pairs = 8
		const perPair = 100

		var wg sync.WaitGroup
		for p := range pairs {
			wg.Add(1)
			go func(p int) {
				defer wg.Done()
				for i := range perPair {
					msgID := fmt.Sprintf("msg%d", p)
					sb.Queue(queueAddEvent(msgID, fmt.Sprintf("p%d-u%d", p, i)))
				}
			}(p)
		}
		wg.Wait()

		// The newest event of every pair must eventually be processed.
		for p := range pairs {
			Eventually(func() bool {
				for _, e := range rec.all() {
					if e.MessageID == fmt.Sprintf("msg%d", p) &&
						e.UserID == fmt.Sprintf("p%d-u%d", p, perPair-1) {
						return true
					}
				}
				return false
			}).Should(BeTrue(), fmt.Sprintf("pair %d's newest event was dropped", p))
		}

		// Per-message processing order is a subsequence of the enqueue order.
		for p := range pairs {
			last := -1
			for _, e := range rec.all() {
				if e.MessageID != fmt.Sprintf("msg%d", p) {
					continue
				}
				idx := 0
				fmt.Sscanf(e.UserID, fmt.Sprintf("p%d-u%%d", p), &idx)
				Expect(idx).To(BeNumerically(">", last), "per-message order violated")
				last = idx
			}
		}
	})
})
