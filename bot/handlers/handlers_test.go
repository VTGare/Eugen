package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/Eugen/bot/handlers"
	"github.com/VTGare/Eugen/store"
	"github.com/bwmarrin/discordgo"
)

func jsonMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

var errBoom = errors.New("boom")

// configureGuild writes scalar settings for the fixture's guild through the
// store so its in-memory cache stays consistent.
func configureGuild(fx *botFixture, enabled bool, starboard, emote string) {
	patch := store.GuildPatch{
		Enabled:          &enabled,
		StarboardChannel: &starboard,
		StarEmote:        &emote,
	}
	Expect(fx.b.Store.Guilds.Update(context.Background(), "guild1", patch)).To(Succeed())
}

var _ = Describe("MessageCreate handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
	})

	It("ignores messages from bots", func() {
		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
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

		Expect(requests.Load()).To(BeZero(), "bot messages should be ignored")
	})

	It("returns early when prefix does not match", func() {
		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
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

		Expect(requests.Load()).To(BeZero(), "messages without prefix should be ignored")
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
		var requests atomic.Int32
		var postPath string
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
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

		Eventually(func() int32 { return requests.Load() }).Should(BeNumerically(">=", 1))
		Expect(postPath).To(ContainSubstring("/channels/chan1/messages"), "should send error message to channel")
	})

	It("sends error message when command exec fails", func() {
		fx.addTestGuild("guild1", "Test Guild")
		var requests atomic.Int32
		fx.b.Registry.Groups()[0].Commands["ping"].Exec = func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
			return errBoom
		}

		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
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

		Eventually(func() int32 { return requests.Load() }).Should(BeNumerically(">=", 1))
	})
})

var _ = Describe("MessageReactionAdd handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
		fx.addTestGuild("guild1", "Test Guild")
	})

	It("returns early when guild is not in cache", func() {
		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "nonexistent", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(requests.Load()).To(BeZero(), "no message fetch should happen for uncached guild")
	})

	It("returns early when guild is disabled", func() {
		configureGuild(fx, false, "", "")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("returns early when starboard channel is empty", func() {
		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("returns early when emoji does not match", func() {
		configureGuild(fx, true, "starboard1", "\u2b50")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "999", Name: "thumbsup"},
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("returns early when channel is banned", func() {
		configureGuild(fx, true, "starboard1", "\u2b50")
		Expect(fx.b.Store.Guilds.BanChannel(context.Background(), "guild1", "bannedchan")).To(Succeed())

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionAdd(fx.b)(fx.sess.Session, &discordgo.MessageReactionAdd{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "bannedchan", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("fetches the message when all checks pass", func() {
		configureGuild(fx, true, "starboard1", "\u2b50")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
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

		Expect(requests.Load()).To(BeNumerically(">=", 1), "message should be fetched when all checks pass")
	})
})

var _ = Describe("MessageReactionRemove handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
		fx.addTestGuild("guild1", "Test Guild")
	})

	It("returns early when guild is not in cache", func() {
		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionRemove(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemove{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "nonexistent", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("returns early when guild is disabled", func() {
		configureGuild(fx, false, "", "")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionRemove(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemove{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "1", Name: "star"},
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("returns early when emoji does not match", func() {
		configureGuild(fx, true, "starboard1", "\u2b50")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionRemove(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemove{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "guild1", ChannelID: "chan1", MessageID: "msg1",
				Emoji: discordgo.Emoji{ID: "999", Name: "thumbsup"},
			},
		})

		Expect(requests.Load()).To(BeZero())
	})
})

var _ = Describe("MessageReactionRemoveAll handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
		fx.addTestGuild("guild1", "Test Guild")
	})

	It("returns early when guild is not in cache", func() {
		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageReactionRemoveAll(fx.b)(fx.sess.Session, &discordgo.MessageReactionRemoveAll{
			MessageReaction: &discordgo.MessageReaction{
				GuildID: "nonexistent", ChannelID: "chan1", MessageID: "msg1",
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("does not call Starboarder when guild is disabled", func() {
		configureGuild(fx, false, "starboard1", "")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
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

		Eventually(func() int32 { return requests.Load() }).Should(BeNumerically(">=", 1), "message should be fetched")
	})

	It("does not call Starboarder when starboard channel is empty", func() {
		configureGuild(fx, true, "", "")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
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

		Eventually(func() int32 { return requests.Load() }).Should(BeNumerically(">=", 1), "message should be fetched")
	})

	It("does not call Starboarder when message author is the bot", func() {
		configureGuild(fx, true, "starboard1", "")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
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

		Eventually(func() int32 { return requests.Load() }).Should(BeNumerically(">=", 1))
	})
})

var _ = Describe("MessageDelete handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
		fx.addTestGuild("guild1", "Test Guild")
	})

	It("returns early when guild is not in cache", func() {
		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageDelete(fx.b)(fx.sess.Session, &discordgo.MessageDelete{
			Message: &discordgo.Message{
				GuildID: "nonexistent", ChannelID: "chan1", ID: "msg1",
			},
		})

		Expect(requests.Load()).To(BeZero(), "no API calls for uncached guild")
	})

	It("returns early when guild is disabled", func() {
		configureGuild(fx, false, "", "")

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageDelete(fx.b)(fx.sess.Session, &discordgo.MessageDelete{
			Message: &discordgo.Message{
				GuildID: "guild1", ChannelID: "chan1", ID: "msg1",
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("returns early when starboard channel is empty", func() {
		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageDelete(fx.b)(fx.sess.Session, &discordgo.MessageDelete{
			Message: &discordgo.Message{
				GuildID: "guild1", ChannelID: "chan1", ID: "msg1",
			},
		})

		Expect(requests.Load()).To(BeZero())
	})

	It("returns early when channel is banned", func() {
		configureGuild(fx, true, "starboard1", "")
		Expect(fx.b.Store.Guilds.BanChannel(context.Background(), "guild1", "chan1")).To(Succeed())

		var requests atomic.Int32
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Message{ID: "msg1"}), 200
		})

		handlers.MessageDelete(fx.b)(fx.sess.Session, &discordgo.MessageDelete{
			Message: &discordgo.Message{
				GuildID: "guild1", ChannelID: "chan1", ID: "msg1",
			},
		})

		Expect(requests.Load()).To(BeZero())
	})
})

var _ = Describe("GuildCreate handler", func() {
	var fx *botFixture

	BeforeEach(func() {
		fx = newBotFixture()
	})

	It("does nothing when guild already in cache", func() {
		fx.addTestGuild("guild1", "Test Guild")
		var requests atomic.Int32
		fx.sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
			requests.Add(1)
			return jsonMarshal(&discordgo.Guild{ID: "guild1"}), 200
		})

		handlers.GuildCreate(fx.b)(fx.sess.Session, &discordgo.GuildCreate{
			Guild: &discordgo.Guild{ID: "guild1", Name: "Test Guild"},
		})

		Expect(requests.Load()).To(BeZero(), "no DB writes for cached guild")
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
