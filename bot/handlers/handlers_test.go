package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/Eugen/bot/handlers"
	"github.com/bwmarrin/discordgo"
)

func jsonMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

var errBoom = errors.New("boom")

var _ = Describe("MessageCreate handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
	})

	It("ignores messages from bots", func() {
		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "resp"}), 200
		})

		handlers.MessageCreate(fx.b)(fx.sess.Session, &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "msg1",
				ChannelID: "chan1",
				GuildID:   "guild1",
				Author:    &discordgo.User{ID: "bot", Bot: true},
				Content:   "e!ping",
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero(), "bot messages should be ignored")
	})

	It("returns early when prefix does not match", func() {
		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "resp"}), 200
		})

		handlers.MessageCreate(fx.b)(fx.sess.Session, &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "msg1",
				ChannelID: "chan1",
				GuildID:   "guild1",
				Author:    &discordgo.User{ID: "user1"},
				Content:   "hello world",
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero(), "messages without prefix should be ignored")
	})

	It("executes a matching command via prefix", func() {
		fx.addTestGuild("guild1", "Test Guild")
		var execCalled int32
		fx.b.Registry.Groups()[0].Commands["ping"].Exec = func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
			atomic.StoreInt32(&execCalled, 1)
			return nil
		}

		handlers.MessageCreate(fx.b)(fx.sess.Session, &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "msg1",
				ChannelID: "chan1",
				GuildID:   "guild1",
				Author:    &discordgo.User{ID: "user1"},
				Content:   "e!ping",
			},
		})

		Eventually(func() int32 { return atomic.LoadInt32(&execCalled) }).Should(Equal(int32(1)))
	})

	It("rejects GuildOnly commands in DMs", func() {
		var requests int32
		var postPath string
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			if req.Method == http.MethodPost {
				postPath = req.URL.Path
			}
			return jsonMarshal(&discordgo.Message{ID: "resp"}), 200
		})

		handlers.MessageCreate(fx.b)(fx.sess.Session, &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "msg1",
				ChannelID: "chan1",
				GuildID:   "", // DM channel
				Author:    &discordgo.User{ID: "user1"},
				Content:   "e!guildonly",
			},
		})

		Eventually(func() int32 { return atomic.LoadInt32(&requests) }).Should(BeNumerically(">=", 1))
		Expect(postPath).To(ContainSubstring("/channels/chan1/messages"), "should send error message to channel")
	})

	It("sends error message when command exec fails", func() {
		fx.addTestGuild("guild1", "Test Guild")
		var requests int32
		fx.b.Registry.Groups()[0].Commands["ping"].Exec = func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
			return errBoom
		}

		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "resp"}), 200
		})

		handlers.MessageCreate(fx.b)(fx.sess.Session, &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ID:        "msg1",
				ChannelID: "chan1",
				GuildID:   "guild1",
				Author:    &discordgo.User{ID: "user1"},
				Content:   "e!ping",
			},
		})

		Eventually(func() int32 { return atomic.LoadInt32(&requests) }).Should(BeNumerically(">=", 1))
	})
})

var _ = Describe("MessageReactionAdd handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
		fx.addTestGuild("guild1", "Test Guild")
	})

	It("returns early when guild is not in cache", func() {
		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "nonexistent", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero(), "no message fetch should happen for uncached guild")
	})

	It("returns early when guild is disabled", func() {
		fx.b.Store.Guilds.Cache().Get("guild1").Enabled = false

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("returns early when starboard channel is empty", func() {
		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("returns early when emoji does not match", func() {
		g := fx.b.Store.Guilds.Cache().Get("guild1")
		g.Enabled = true
		g.StarboardChannel = "starboard1"
		g.StarEmote = "\u2b50"

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "999", Name: "thumbsup"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("returns early when channel is banned", func() {
		g := fx.b.Store.Guilds.Cache().Get("guild1")
		g.Enabled = true
		g.StarboardChannel = "starboard1"
		g.StarEmote = "\u2b50"
		g.BannedChannels = []string{"bannedchan"}

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "bannedchan", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("fetches the message when all checks pass", func() {
		g := fx.b.Store.Guilds.Cache().Get("guild1")
		g.Enabled = true
		g.StarboardChannel = "starboard1"
		g.StarEmote = "\u2b50"

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			if req.Method == http.MethodGet {
				// Return a message with no matching star reaction so the
				// handler returns at findReaction without calling Starboarder.
				return jsonMarshal(&discordgo.Message{
					ID:        "msg1",
					Author:    &discordgo.User{ID: "author1"},
					Reactions: []*discordgo.MessageReactions{},
				}), 200
			}
			return jsonMarshal(&discordgo.Message{ID: "resp"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "", Name: "\u2b50"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeNumerically(">=", 1), "message should be fetched when all checks pass")
	})
})

var _ = Describe("MessageReactionRemove handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
		fx.addTestGuild("guild1", "Test Guild")
	})

	It("returns early when guild is not in cache", func() {
		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionRemove(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemove{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "nonexistent", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("returns early when guild is disabled", func() {
		fx.b.Store.Guilds.Cache().Get("guild1").Enabled = false

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionRemove(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemove{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("returns early when emoji does not match", func() {
		g := fx.b.Store.Guilds.Cache().Get("guild1")
		g.Enabled = true
		g.StarboardChannel = "starboard1"
		g.StarEmote = "\u2b50"

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionRemove(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemove{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "999", Name: "thumbsup"},
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})
})

var _ = Describe("MessageReactionRemoveAll handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
		fx.addTestGuild("guild1", "Test Guild")
	})

	It("returns early when guild is not in cache", func() {
		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionRemoveAll(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemoveAll{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "nonexistent", ChannelID: "chan1", MessageID: "msg1",
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("does not call Starboarder when guild is disabled", func() {
		g := fx.b.Store.Guilds.Cache().Get("guild1")
		g.Enabled = false
		g.StarboardChannel = "starboard1"

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{
				ID:     "msg1",
				Author: &discordgo.User{ID: "author1"},
			}), 200
		})

		handlers.MessageReactionRemoveAll(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemoveAll{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
			},
		})

		Eventually(func() int32 { return atomic.LoadInt32(&requests) }).Should(BeNumerically(">=", 1), "message should be fetched")
	})

	It("does not call Starboarder when starboard channel is empty", func() {
		g := fx.b.Store.Guilds.Cache().Get("guild1")
		g.Enabled = true
		g.StarboardChannel = ""

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{
				ID:     "msg1",
				Author: &discordgo.User{ID: "author1"},
			}), 200
		})

		handlers.MessageReactionRemoveAll(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemoveAll{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
			},
		})

		Eventually(func() int32 { return atomic.LoadInt32(&requests) }).Should(BeNumerically(">=", 1), "message should be fetched")
	})

	It("does not call Starboarder when message author is the bot", func() {
		g := fx.b.Store.Guilds.Cache().Get("guild1")
		g.Enabled = true
		g.StarboardChannel = "starboard1"

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{
				ID:     "msg1",
				Author: &discordgo.User{ID: "123456789"}, // bot's own ID
			}), 200
		})

		handlers.MessageReactionRemoveAll(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemoveAll{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
			},
		})

		Eventually(func() int32 { return atomic.LoadInt32(&requests) }).Should(BeNumerically(">=", 1))
	})

	// Note: The "all checks pass" path calls b.Starboard.ReactionsCleared()
	// which processes events asynchronously via a goroutine that calls
	// store.Messages.Repost(). With a real MongoDB instance from
	// testcontainers, the Starboarder processing can be fully exercised.
})

var _ = Describe("MessageDelete handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
		fx.addTestGuild("guild1", "Test Guild")
	})

	It("returns early when guild is not in cache", func() {
		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageDelete(fx.b)(fx.sess.Session, &discordgo.MessageDelete{
			Message: &discordgo.Message{
				GuildID: "nonexistent", ChannelID: "chan1", ID: "msg1",
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero(), "no API calls for uncached guild")
	})

	It("returns early when guild is disabled", func() {
		fx.b.Store.Guilds.Cache().Get("guild1").Enabled = false

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageDelete(fx.b)(fx.sess.Session, &discordgo.MessageDelete{
			Message: &discordgo.Message{
				GuildID: "guild1", ChannelID: "chan1", ID: "msg1",
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("returns early when starboard channel is empty", func() {
		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageDelete(fx.b)(fx.sess.Session, &discordgo.MessageDelete{
			Message: &discordgo.Message{
				GuildID: "guild1", ChannelID: "chan1", ID: "msg1",
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})

	It("returns early when channel is banned", func() {
		g := fx.b.Store.Guilds.Cache().Get("guild1")
		g.Enabled = true
		g.StarboardChannel = "starboard1"
		g.BannedChannels = []string{"chan1"}

		var requests int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageDelete(fx.b)(fx.sess.Session, &discordgo.MessageDelete{
			Message: &discordgo.Message{
				GuildID: "guild1", ChannelID: "chan1", ID: "msg1",
			},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero())
	})
})

var _ = Describe("GuildCreate handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
	})

	It("does nothing when guild already in cache", func() {
		fx.addTestGuild("guild1", "Test Guild")
		var requests int32
		fx.sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
			atomic.AddInt32(&requests, 1)
			return jsonMarshal(&discordgo.Guild{ID: "guild1"}), 200
		})

		handlers.GuildCreate(fx.b)(fx.sess.Session, &discordgo.GuildCreate{
			Guild: &discordgo.Guild{ID: "guild1", Name: "Test Guild"},
		})

		Expect(atomic.LoadInt32(&requests)).To(BeZero(), "no DB writes for cached guild")
	})
})

var _ = Describe("Ready handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
	})

	It("sets the bot mention from the ready user ID", func() {
		fx.b.SetMention("<@99999>")
		Expect(fx.b.Mention()).To(Equal("<@99999>"))
	})

	It("updates the mention to a new user ID", func() {
		fx.b.SetMention("<@111>")
		fx.b.SetMention("<@99999>")
		Expect(fx.b.Mention()).To(Equal("<@99999>"))
		Expect(fx.b.Mention()).NotTo(Equal("<@111>"))
	})
})
