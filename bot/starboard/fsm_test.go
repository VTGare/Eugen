package starboard_test

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/VTGare/Eugen/bot/starboard"
	"github.com/VTGare/Eugen/store"
	"github.com/VTGare/Eugen/testutil"
	"github.com/bwmarrin/discordgo"
)

const (
	fxGuildID = "guild1"
	fxChannel = "chan1"
	fxMessage = "msg1"
	fxSBChan  = "sb1"
	fxSBMsg   = "sb1msg"
	fxAuthor  = "author1"
	fxUser    = "reactor1"
)

type sbFixture struct {
	st    *store.Store
	sess  *testutil.MockSession
	sb    *starboard.Starboarder
	guild *store.Guild

	originalGETs atomic.Int32
}

func newSbFixture() *sbFixture {
	st := mongoContainer.NewTestStore(GinkgoT())
	log := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	sess := testutil.NewSession(GinkgoT())
	sess.WithBotUser("123456789", "Eugen")

	guild := store.NewGuild("Test Guild", fxGuildID)
	guild.StarboardChannel = fxSBChan
	guild.MinimumStars = 3
	Expect(st.Guilds.Create(context.Background(), guild)).To(Succeed())

	sb := starboard.New(context.Background(), sess.Session, st, log)

	DeferCleanup(func() {
		sb.WaitForIdle()
		sess.Close()
	})

	return &sbFixture{st: st, sess: sess, sb: sb, guild: guild}
}

// star returns a MessageReactions with the given count for the guild emote.
func star(count int) []*discordgo.MessageReactions {
	return []*discordgo.MessageReactions{{
		Count: count,
		Emoji: &discordgo.Emoji{ID: "", Name: "⭐"},
	}}
}

// mockOriginal serves GET /channels/chan1/messages/msg1 with the given author
// and reaction state, and counts the requests.
func (f *sbFixture) mockOriginal(authorID string, bots bool, reactions []*discordgo.MessageReactions) {
	f.sess.On("/channels/"+fxChannel+"/messages/"+fxMessage, func(req *http.Request) ([]byte, int) {
		f.originalGETs.Add(1)
		if req.Method != http.MethodGet {
			return nil, 405
		}
		return testutil.JSON(&discordgo.Message{
			ID:        fxMessage,
			ChannelID: fxChannel,
			GuildID:   fxGuildID,
			Author:    &discordgo.User{ID: authorID, Bot: bots},
			Content:   "hello starboard",
			Reactions: reactions,
		}, 200)(req)
	})
}

func (f *sbFixture) mockCreateStarboard(posts *atomic.Int32) {
	f.sess.On("/channels/"+fxSBChan+"/messages", func(req *http.Request) ([]byte, int) {
		switch req.Method {
		case http.MethodPost:
			if posts != nil {
				posts.Add(1)
			}
			return testutil.JSON(&discordgo.Message{ID: fxSBMsg, ChannelID: fxSBChan}, 200)(req)
		case http.MethodPatch:
			return testutil.JSON(&discordgo.Message{ID: fxSBMsg, ChannelID: fxSBChan}, 200)(req)
		case http.MethodDelete:
			return nil, 204
		}
		return nil, 405
	})
}

// mockStarboardMessage serves GET /channels/sb1/messages/sb1msg for footer
// updates.
func (f *sbFixture) mockStarboardMessage() {
	f.sess.On("/channels/"+fxSBChan+"/messages/"+fxSBMsg, func(req *http.Request) ([]byte, int) {
		if req.Method != http.MethodGet {
			return nil, 405
		}
		return testutil.JSON(&discordgo.Message{
			ID:        fxSBMsg,
			ChannelID: fxSBChan,
			Embeds:    []*discordgo.MessageEmbed{{Title: "starboard"}},
		}, 200)(req)
	})
}

func (f *sbFixture) mockChannel() {
	f.sess.On("/channels/"+fxChannel, func(req *http.Request) ([]byte, int) {
		if req.Method != http.MethodGet {
			return nil, 405
		}
		return testutil.JSON(&discordgo.Channel{ID: fxChannel, Name: "general"}, 200)(req)
	})
}

func (f *sbFixture) seedRecord() *store.Message {
	oPair := store.NewPair(fxChannel, fxMessage)
	sPair := store.NewPair(fxSBChan, fxSBMsg)
	rec := store.NewMessage(&oPair, &sPair, fxGuildID)
	Expect(f.st.Messages.Create(context.Background(), rec)).To(Succeed())
	return rec
}

func (f *sbFixture) record() *store.Message {
	rec, err := f.st.Messages.GetByOriginal(context.Background(), fxChannel, fxMessage)
	Expect(err).NotTo(HaveOccurred())
	return rec
}

func addEvent() starboard.Event {
	return starboard.Event{
		Type:      starboard.EventReactionAdd,
		ChannelID: fxChannel,
		MessageID: fxMessage,
		GuildID:   fxGuildID,
		UserID:    fxUser,
	}
}

func removeEvent() starboard.Event {
	return starboard.Event{
		Type:      starboard.EventReactionRemove,
		ChannelID: fxChannel,
		MessageID: fxMessage,
		GuildID:   fxGuildID,
		UserID:    fxUser,
	}
}

var _ = Describe("Starboarder processing", func() {
	var fx *sbFixture

	BeforeEach(func() {
		fx = newSbFixture()
	})

	It("creates a starboard post when a message reaches the threshold", func() {
		fx.mockOriginal(fxAuthor, false, star(3))
		fx.mockChannel()
		var posts atomic.Int32
		fx.mockCreateStarboard(&posts)

		fx.sb.ReactionAdd(addEvent())

		Eventually(fx.record).ShouldNot(BeNil(), "a record must be created")
		Expect(posts.Load()).To(BeNumerically("==", 1))
		rec := fx.record()
		Expect(rec.Original).To(Equal(&store.MessagePair{ChannelID: fxChannel, MessageID: fxMessage}))
		Expect(rec.Starboard).To(Equal(&store.MessagePair{ChannelID: fxSBChan, MessageID: fxSBMsg}))
	})

	It("does not create a starboard below the threshold", func() {
		fx.mockOriginal(fxAuthor, false, star(2))
		fx.mockChannel()
		var posts atomic.Int32
		fx.mockCreateStarboard(&posts)

		fx.sb.ReactionAdd(addEvent())

		Eventually(func() int32 { return fx.originalGETs.Load() }).Should(BeNumerically(">=", 1),
			"the worker must process the event")
		Consistently(fx.record).Should(BeNil())
		Expect(posts.Load()).To(BeZero())
	})

	It("does not starboard bot-authored messages", func() {
		fx.mockOriginal("123456789", true, star(10))
		fx.mockChannel()
		var posts atomic.Int32
		fx.mockCreateStarboard(&posts)

		fx.sb.ReactionAdd(addEvent())

		Eventually(func() int32 { return fx.originalGETs.Load() }).Should(BeNumerically(">=", 1))
		Consistently(fx.record).Should(BeNil())
		Expect(posts.Load()).To(BeZero())
	})

	It("excludes self-stars when the guild disallows them", func() {
		Expect(fx.st.Guilds.Update(context.Background(), fxGuildID, store.GuildPatch{Selfstar: lo.ToPtr(false)})).To(Succeed())
		fx.mockOriginal(fxAuthor, false, star(3))
		fx.mockChannel()
		var posts atomic.Int32
		fx.mockCreateStarboard(&posts)

		selfStar := addEvent()
		selfStar.UserID = fxAuthor
		fx.sb.ReactionAdd(selfStar)

		Eventually(func() int32 { return fx.originalGETs.Load() }).Should(BeNumerically(">=", 1))
		Consistently(fx.record).Should(BeNil(), "3 stars minus a self-star is below the threshold")
		Expect(posts.Load()).To(BeZero())
	})

	It("updates the footer when an existing starboard gains reactions", func() {
		fx.seedRecord()
		fx.mockOriginal(fxAuthor, false, star(4))
		fx.mockStarboardMessage()
		var edits atomic.Int32
		fx.sess.On("/channels/"+fxSBChan+"/messages/"+fxSBMsg, func(req *http.Request) ([]byte, int) {
			if req.Method == http.MethodPatch {
				edits.Add(1)
				return testutil.JSON(&discordgo.Message{ID: fxSBMsg, ChannelID: fxSBChan}, 200)(req)
			}
			return testutil.JSON(&discordgo.Message{
				ID:        fxSBMsg,
				ChannelID: fxSBChan,
				Embeds:    []*discordgo.MessageEmbed{{Title: "starboard"}},
			}, 200)(req)
		})

		fx.sb.ReactionAdd(addEvent())

		Eventually(func() int32 { return edits.Load() }).Should(BeNumerically("==", 1))
		Expect(fx.record()).NotTo(BeNil(), "the record must survive footer updates")
	})

	It("removes the starboard when reactions drop to half the threshold", func() {
		fx.seedRecord()
		fx.mockOriginal(fxAuthor, false, star(1)) // 3/2 = 1
		var deletes atomic.Int32
		fx.sess.On("/channels/"+fxSBChan+"/messages/"+fxSBMsg, func(req *http.Request) ([]byte, int) {
			if req.Method == http.MethodDelete {
				deletes.Add(1)
				return nil, 204
			}
			return nil, 405
		})

		fx.sb.ReactionRemove(removeEvent())

		Eventually(func() int32 { return deletes.Load() }).Should(BeNumerically("==", 1))
		Eventually(fx.record).Should(BeNil(), "the record must be removed")
	})

	It("converges: a remove event creates a starboard when the count is at threshold", func() {
		fx.mockOriginal(fxAuthor, false, star(4))
		fx.mockChannel()
		var posts atomic.Int32
		fx.mockCreateStarboard(&posts)

		fx.sb.ReactionRemove(removeEvent())

		Eventually(fx.record).ShouldNot(BeNil(), "both reaction events converge on the same state machine")
		Expect(posts.Load()).To(BeNumerically("==", 1))
	})

	It("removes the record when reactions are cleared", func() {
		fx.seedRecord()
		fx.mockOriginal(fxAuthor, false, star(0))
		var deletes atomic.Int32
		fx.sess.On("/channels/"+fxSBChan+"/messages/"+fxSBMsg, func(req *http.Request) ([]byte, int) {
			if req.Method == http.MethodDelete {
				deletes.Add(1)
				return nil, 204
			}
			return nil, 405
		})

		fx.sb.ReactionsCleared(starboard.Event{
			Type:      starboard.EventReactionsClear,
			ChannelID: fxChannel,
			MessageID: fxMessage,
			GuildID:   fxGuildID,
		})

		Eventually(func() int32 { return deletes.Load() }).Should(BeNumerically("==", 1))
		Eventually(fx.record).Should(BeNil())
	})

	It("keeps the starboard when reactions are cleared on the bot's own message", func() {
		fx.seedRecord()
		fx.mockOriginal("123456789", true, star(0))

		fx.sb.ReactionsCleared(starboard.Event{
			Type:      starboard.EventReactionsClear,
			ChannelID: fxChannel,
			MessageID: fxMessage,
			GuildID:   fxGuildID,
		})

		time.Sleep(150 * time.Millisecond)
		Expect(fx.record()).NotTo(BeNil(), "clearing the bot's own starboard must not delete it")
	})

	It("deleting the starboard post removes the record", func() {
		fx.seedRecord()
		var deletes atomic.Int32
		fx.sess.On("/channels/"+fxSBChan+"/messages/"+fxSBMsg, func(req *http.Request) ([]byte, int) {
			if req.Method == http.MethodDelete {
				deletes.Add(1)
				return nil, 204
			}
			return nil, 405
		})

		fx.sb.MessageDeleted(starboard.Event{
			Type:           starboard.EventMessageDelete,
			ChannelID:      fxSBChan,
			MessageID:      fxSBMsg,
			GuildID:        fxGuildID,
			DeletedChannel: fxSBChan,
			DeletedMessage: fxSBMsg,
		})

		Eventually(fx.record).Should(BeNil(), "the record must be removed")
		Expect(deletes.Load()).To(BeZero(),
			"the post is already gone; no extra delete call may be issued")
		Expect(fx.originalGETs.Load()).To(BeZero(), "no original message fetch needed")
	})

	It("deleting the original message removes the record, deletes the post and freezes the lane", func() {
		fx.seedRecord()
		fx.mockOriginal(fxAuthor, false, star(9)) // must never be fetched
		var deletes atomic.Int32
		fx.sess.On("/channels/"+fxSBChan+"/messages/"+fxSBMsg, func(req *http.Request) ([]byte, int) {
			if req.Method == http.MethodDelete {
				deletes.Add(1)
				return nil, 204
			}
			return nil, 405
		})

		fx.sb.MessageDeleted(starboard.Event{
			Type:           starboard.EventMessageDelete,
			ChannelID:      fxChannel,
			MessageID:      fxMessage,
			GuildID:        fxGuildID,
			DeletedChannel: fxChannel,
			DeletedMessage: fxMessage,
		})
		fx.sb.ReactionAdd(addEvent())

		Eventually(func() int32 { return deletes.Load() }).Should(BeNumerically("==", 1),
			"the starboard post must be deleted when the original message is deleted")
		Eventually(fx.record).Should(BeNil(), "the record must be removed")
		time.Sleep(150 * time.Millisecond)
		Expect(fx.originalGETs.Load()).To(BeZero(),
			"reaction events after an original-message delete must be dropped")
	})

	It("ignores deletes of messages that are not starboarded", func() {
		fx.sb.MessageDeleted(starboard.Event{
			Type:           starboard.EventMessageDelete,
			ChannelID:      "chan2",
			MessageID:      "msg2",
			GuildID:        fxGuildID,
			DeletedChannel: "chan2",
			DeletedMessage: "msg2",
		})

		time.Sleep(150 * time.Millisecond)
		Expect(fx.record()).To(BeNil())
		Expect(fx.originalGETs.Load()).To(BeZero(), "unrelated deletes must be dropped at queue time")
	})
})
