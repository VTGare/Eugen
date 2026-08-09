package commands_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/Eugen/bot/commands"
	"github.com/bwmarrin/discordgo"
)

var _ = Describe("ping command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
	})

	It("responds with embed containing heartbeat latency", func() {
		// Register a handler to capture sent embeds.
		var capturedBody []byte
		var capturedStatus int
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			if req.Method == "POST" {
				capturedBody = readBody(req)
			}
			capturedStatus = 200
			m := &discordgo.Message{ID: "resp1", ChannelID: "200"}
			return jsonMarshal(m), 200
		})

		cmd := fx.b.Registry.Get("ping")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1", Username: "test"},
				Content:   "e!ping",
			},
		}
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).NotTo(HaveOccurred())

		// Verify the embed was sent.
		Expect(capturedBody).NotTo(BeNil())
		Expect(string(capturedBody)).To(ContainSubstring("Pong"))
		_ = capturedStatus
	})
})

var _ = Describe("help command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
	})

	It("lists all visible commands when no args provided", func() {
		var capturedBody []byte
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			if req.Method == "POST" {
				capturedBody = readBody(req)
			}
			m := &discordgo.Message{ID: "resp1", ChannelID: "200"}
			return jsonMarshal(m), 200
		})

		cmd := fx.b.Registry.Get("help")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
				Content:   "e!help",
			},
		}
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(capturedBody)).To(ContainSubstring("Commands"))
		Expect(string(capturedBody)).To(ContainSubstring("ping"))
	})

	It("shows extended help for a specific command", func() {
		var capturedBody []byte
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			if req.Method == "POST" {
				capturedBody = readBody(req)
			}
			m := &discordgo.Message{ID: "resp1", ChannelID: "200"}
			return jsonMarshal(m), 200
		})

		cmd := fx.b.Registry.Get("help")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
				Content:   "e!help",
			},
		}
		err := cmd.Exec(fx.sess.Session, m, []string{"ping"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(capturedBody)).To(ContainSubstring("help"))
	})

	It("returns error for too many arguments", func() {
		cmd := fx.b.Registry.Get("help")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, m, []string{"one", "two"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("incorrect command usage"))
	})

	It("shows error for unknown command name", func() {
		var capturedBody []byte
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			if req.Method == "POST" {
				capturedBody = readBody(req)
			}
			m := &discordgo.Message{ID: "resp1", ChannelID: "200"}
			return jsonMarshal(m), 200
		})

		cmd := fx.b.Registry.Get("help")
		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, m, []string{"nonexistent"})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(capturedBody)).To(ContainSubstring("Unknown command"))
	})
})

var _ = Describe("invite command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
	})

	It("sends an invite link embed", func() {
		var capturedBody []byte
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			if req.Method == "POST" {
				capturedBody = readBody(req)
			}
			m := &discordgo.Message{ID: "resp1", ChannelID: "200"}
			return jsonMarshal(m), 200
		})

		cmd := fx.b.Registry.Get("invite")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
				Content:   "e!invite",
			},
		}
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(capturedBody)).To(ContainSubstring("discord.com/api/oauth2/authorize"))
	})
})

var _ = Describe("ban command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
		fx.addTestGuild("100", "Test Guild")
		fx.addTestMember("100", "user1")
	})

	It("returns permission error for non-admin", func() {
		// Override the member's roles to have no admin.
		m := &discordgo.Member{
			GuildID: "100",
			User:    &discordgo.User{ID: "user1", Username: "test"},
			Roles:   []string{},
		}
		_ = fx.sess.Session.State.MemberAdd(m)

		fx.sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
			g := &discordgo.Guild{
				ID:      "100",
				Name:    "Test Guild",
				OwnerID: "999",
				Roles:   []*discordgo.Role{{ID: "role1", Permissions: 0}},
			}
			return jsonMarshal(g), 200
		})

		cmd := fx.b.Registry.Get("ban")
		Expect(cmd).NotTo(BeNil())

		msg := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, msg, []string{"general"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("permission"))
	})

	It("returns not enough arguments for empty args", func() {
		cmd := fx.b.Registry.Get("ban")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).To(MatchError(commands.ErrNotEnoughArguments))
	})
})

var _ = Describe("unban command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
		fx.addTestGuild("100", "Test Guild")
		fx.addTestMember("100", "user1")
	})

	It("returns permission error for non-admin", func() {
		m := &discordgo.Member{
			GuildID: "100",
			User:    &discordgo.User{ID: "user1", Username: "test"},
			Roles:   []string{},
		}
		_ = fx.sess.Session.State.MemberAdd(m)

		fx.sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
			g := &discordgo.Guild{
				ID:      "100",
				Name:    "Test Guild",
				OwnerID: "999",
				Roles:   []*discordgo.Role{{ID: "role1", Permissions: 0}},
			}
			return jsonMarshal(g), 200
		})

		cmd := fx.b.Registry.Get("unban")
		Expect(cmd).NotTo(BeNil())

		msg := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, msg, []string{"general"})
		Expect(err).To(HaveOccurred())
	})

	It("returns not enough arguments for empty args", func() {
		cmd := fx.b.Registry.Get("unban")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).To(MatchError(commands.ErrNotEnoughArguments))
	})
})

var _ = Describe("blacklist command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
		fx.addTestGuild("100", "Test Guild")
		fx.addTestMember("100", "user1")
	})

	It("returns permission error for non-admin", func() {
		m := &discordgo.Member{
			GuildID: "100",
			User:    &discordgo.User{ID: "user1", Username: "test"},
			Roles:   []string{},
		}
		_ = fx.sess.Session.State.MemberAdd(m)

		fx.sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
			g := &discordgo.Guild{
				ID:      "100",
				Name:    "Test Guild",
				OwnerID: "999",
				Roles:   []*discordgo.Role{{ID: "role1", Permissions: 0}},
			}
			return jsonMarshal(g), 200
		})

		cmd := fx.b.Registry.Get("blacklist")
		Expect(cmd).NotTo(BeNil())

		msg := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, msg, []string{"user"})
		Expect(err).To(HaveOccurred())
	})

	It("returns not enough arguments for empty args", func() {
		cmd := fx.b.Registry.Get("blacklist")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).To(MatchError(commands.ErrNotEnoughArguments))
	})
})

var _ = Describe("unblacklist command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
		fx.addTestGuild("100", "Test Guild")
		fx.addTestMember("100", "user1")
	})

	It("returns permission error for non-admin", func() {
		m := &discordgo.Member{
			GuildID: "100",
			User:    &discordgo.User{ID: "user1", Username: "test"},
			Roles:   []string{},
		}
		_ = fx.sess.Session.State.MemberAdd(m)

		fx.sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
			g := &discordgo.Guild{
				ID:      "100",
				Name:    "Test Guild",
				OwnerID: "999",
				Roles:   []*discordgo.Role{{ID: "role1", Permissions: 0}},
			}
			return jsonMarshal(g), 200
		})

		cmd := fx.b.Registry.Get("unblacklist")
		Expect(cmd).NotTo(BeNil())

		msg := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, msg, []string{"user"})
		Expect(err).To(HaveOccurred())
	})

	It("returns not enough arguments for empty args", func() {
		cmd := fx.b.Registry.Get("unblacklist")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).To(MatchError(commands.ErrNotEnoughArguments))
	})
})

var _ = Describe("req command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
		fx.addTestGuild("100", "Test Guild")
		fx.addTestMember("100", "user1")
	})

	It("returns permission error for non-admin", func() {
		m := &discordgo.Member{
			GuildID: "100",
			User:    &discordgo.User{ID: "user1", Username: "test"},
			Roles:   []string{},
		}
		_ = fx.sess.Session.State.MemberAdd(m)

		fx.sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
			g := &discordgo.Guild{
				ID:      "100",
				Name:    "Test Guild",
				OwnerID: "999",
				Roles:   []*discordgo.Role{{ID: "role1", Permissions: 0}},
			}
			return jsonMarshal(g), 200
		})

		cmd := fx.b.Registry.Get("req")
		Expect(cmd).NotTo(BeNil())

		msg := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, msg, []string{"general", "5"})
		Expect(err).To(HaveOccurred())
	})

	It("returns not enough arguments for empty args", func() {
		cmd := fx.b.Registry.Get("req")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).To(MatchError(commands.ErrNotEnoughArguments))
	})
})

var _ = Describe("SetChannelStars", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
		fx.addTestGuild("100", "Test Guild")
	})

	It("appends a per-channel star requirement when not configured", func() {
		Expect(fx.b.Store.Guilds.SetChannelStars(context.Background(), "100", "200", 7)).To(Succeed())

		g := fx.b.Store.Guilds.Get(context.Background(), "100")
		Expect(g).NotTo(BeNil())
		Expect(g.StarsRequired("200")).To(Equal(7))
		Expect(g.StarsRequired("999")).To(Equal(5))

		guilds, err := fx.b.Store.Guilds.List(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(guilds).To(HaveLen(1))
		Expect(guilds[0].StarsRequired("200")).To(Equal(7))
	})

	It("updates an existing per-channel star requirement", func() {
		Expect(fx.b.Store.Guilds.SetChannelStars(context.Background(), "100", "200", 7)).To(Succeed())
		Expect(fx.b.Store.Guilds.SetChannelStars(context.Background(), "100", "200", 9)).To(Succeed())

		g := fx.b.Store.Guilds.Get(context.Background(), "100")
		Expect(g).NotTo(BeNil())
		Expect(g.StarsRequired("200")).To(Equal(9))
		Expect(g.ChannelSettings).To(HaveLen(1))
	})
})

var _ = Describe("set command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
		fx.addTestGuild("100", "Test Guild")
		fx.addTestMember("100", "user1")
	})

	Describe("argument handling", func() {
		It("shows guild settings when no args", func() {
			var requests atomic.Int32
			fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
				if req.Method == http.MethodPost {
					requests.Add(1)
				}
				return jsonMarshal(&discordgo.Message{ID: "r1"}), 200
			})

			err := execSet(fx)
			Expect(err).NotTo(HaveOccurred())
			Expect(requests.Load()).To(Equal(int32(1)), "settings embed should be sent")
		})

		It("returns incorrect usage for 1 arg", func() {
			m := &discordgo.MessageCreate{
				Message: &discordgo.Message{
					ChannelID: "200",
					GuildID:   "100",
					Author:    &discordgo.User{ID: "user1"},
				},
			}

			cmd := fx.b.Registry.Get("set")
			Expect(cmd).NotTo(BeNil())

			err := cmd.Exec(fx.sess.Session, m, []string{"withone"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("incorrect command usage"))
		})

		It("returns incorrect usage for 3+ args", func() {
			err := execSet(fx, "enabled", "true", "extra")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("incorrect command usage"))
		})
	})

	Describe("permissions", func() {
		It("returns ErrNoPermission for a non-admin", func() {
			makeNonAdminMember(fx)

			err := execSet(fx, "enabled", "true")
			Expect(err).To(MatchError(commands.ErrNoPermission))
		})

		It("checks permissions before validating the setting name", func() {
			makeNonAdminMember(fx)

			err := execSet(fx, "bogus", "true")
			Expect(err).To(MatchError(commands.ErrNoPermission))
		})
	})

	Describe("unknown setting", func() {
		It("rejects an unknown setting", func() {
			err := execSet(fx, "bogus", "true")
			Expect(err).To(MatchError("unknown setting bogus"))
		})

		It("does not persist anything for an unknown setting", func() {
			var requests atomic.Int32
			fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
				requests.Add(1)
				return jsonMarshal(&discordgo.Message{ID: "r1"}), 200
			})

			err := execSet(fx, "bogus", "true")
			Expect(err).To(HaveOccurred())
			Expect(requests.Load()).To(BeZero(), "no embed should be sent")

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.Prefix).To(Equal("e!"), "guild config should be unchanged")
		})
	})

	Describe("enabled", func() {
		It("sets enabled to true", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "enabled", "true")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "enabled", "true")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.Enabled).To(BeTrue())
		})

		It("sets enabled to false", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "enabled", "false")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "enabled", "false")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.Enabled).To(BeFalse())
		})

		It("rejects a non-boolean value", func() {
			err := execSet(fx, "enabled", "maybe")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to parse"))
		})
	})

	Describe("selfstar", func() {
		It("sets selfstar to true", func() {
			err := execSet(fx, "selfstar", "true")
			Expect(err).NotTo(HaveOccurred())
			Expect(fx.b.Store.Guilds.Get(context.Background(), "100").Selfstar).To(BeTrue())
		})

		It("sets selfstar to false", func() {
			err := execSet(fx, "selfstar", "false")
			Expect(err).NotTo(HaveOccurred())
			Expect(fx.b.Store.Guilds.Get(context.Background(), "100").Selfstar).To(BeFalse())
		})

		It("rejects a non-boolean value", func() {
			err := execSet(fx, "selfstar", "yes")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to parse"))
		})
	})

	Describe("ignorebots", func() {
		It("sets ignorebots to true", func() {
			err := execSet(fx, "ignorebots", "true")
			Expect(err).NotTo(HaveOccurred())
			Expect(fx.b.Store.Guilds.Get(context.Background(), "100").IgnoreBots).To(BeTrue())
		})

		It("rejects a non-boolean value", func() {
			err := execSet(fx, "ignorebots", "yes")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to parse"))
		})
	})

	Describe("color", func() {
		It("sets a decimal color", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "color", "12345")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "color", "12345")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.EmbedColor).To(Equal(int64(12345)))
		})

		It("sets a hexadecimal color", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "color", "ff0000")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "color", "16711680")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.EmbedColor).To(Equal(int64(16711680)))
		})

		It("sets a color with an explicit 0x prefix", func() {
			err := execSet(fx, "color", "0xff")
			Expect(err).NotTo(HaveOccurred())
			Expect(fx.b.Store.Guilds.Get(context.Background(), "100").EmbedColor).To(Equal(int64(255)))
		})

		It("normalizes an uppercase hex color", func() {
			err := execSet(fx, "color", "FF00FF")
			Expect(err).NotTo(HaveOccurred())
			Expect(fx.b.Store.Guilds.Get(context.Background(), "100").EmbedColor).To(Equal(int64(16711935)))
		})

		It("rejects a non-color value", func() {
			err := execSet(fx, "color", "red")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to parse"))
		})
	})

	Describe("prefix", func() {
		It("sets a punctuation prefix as-is", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "prefix", "!")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "prefix", "!")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.Prefix).To(Equal("!"))
		})

		It("appends a trailing space to letter-based prefixes", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "prefix", "test")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "prefix", "test ")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.Prefix).To(Equal("test "))
		})

		It("rejects a prefix longer than 5 characters", func() {
			err := execSet(fx, "prefix", "123456")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prefix is too long"))
		})
	})

	Describe("emote", func() {
		It("resolves a guild emoji to its mention form", func() {
			err := execSet(fx, "emote", "<:AmeliaTopKekA:827607018726359130>")
			Expect(err).NotTo(HaveOccurred())
			Expect(fx.b.Store.Guilds.Get(context.Background(), "100").StarEmote).To(Equal("<:ameliatopkeka:827607018726359130>"))
		})

		It("resolves a guild emoji regardless of case", func() {
			err := execSet(fx, "emote", "<:AmelIATOpKekA:827607018726359130>")
			Expect(err).NotTo(HaveOccurred())
			Expect(fx.b.Store.Guilds.Get(context.Background(), "100").StarEmote).To(Equal("<:ameliatopkeka:827607018726359130>"))
		})

		It("stores a unicode emoji as-is", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "emote", "⭐")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "emote", "⭐")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.StarEmote).To(Equal("⭐"))
		})

		It("rejects an emote when guild emojis can't be loaded", func() {
			fx.sess.On("/guilds/100/emojis", func(req *http.Request) ([]byte, int) {
				return nil, 500
			})

			err := execSet(fx, "emote", "⭐")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("argument's either a global emoji or not one at all"))
		})
	})

	Describe("starboard", func() {
		It("sets the starboard channel from a mention", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "starboard", "<#200>")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "starboard", "<#200>")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.StarboardChannel).To(Equal("200"))
		})

		It("sets the starboard channel from a bare channel id", func() {
			err := execSet(fx, "starboard", "200")
			Expect(err).NotTo(HaveOccurred())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.StarboardChannel).To(Equal("200"))
		})

		It("rejects a channel from a foreign server", func() {
			fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
				if strings.HasSuffix(req.URL.Path, "/999") {
					return jsonMarshal(&discordgo.Channel{ID: "999", Name: "other", GuildID: "200"}), 200
				}
				return jsonMarshal(&discordgo.Channel{ID: "200", Name: "general", GuildID: "100"}), 200
			})

			err := execSet(fx, "starboard", "<#999>")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("foreign server"))
		})

		It("rejects a value that isn't a channel", func() {
			fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
				if strings.HasSuffix(req.URL.Path, "/boguschan") {
					return nil, 404
				}
				return jsonMarshal(&discordgo.Channel{ID: "200", Name: "general", GuildID: "100"}), 200
			})

			err := execSet(fx, "starboard", "boguschan")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("stars", func() {
		It("sets the minimum star count", func() {
			captured := captureChannelPosts(fx)

			err := execSet(fx, "stars", "8")
			Expect(err).NotTo(HaveOccurred())
			Expect(assertSetCommandSuccess(captured, "stars", "8")).To(Succeed())

			g := fx.b.Store.Guilds.Get(context.Background(), "100")
			Expect(g.MinimumStars).To(Equal(8))
		})

		It("rejects a star count below 1", func() {
			err := execSet(fx, "stars", "0")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("minimum stars must be >= 1"))
		})

		It("rejects a non-integer star count", func() {
			err := execSet(fx, "stars", "many")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to parse"))
		})
	})
})

// execSet runs the set command with the given arguments from the fixture's
// default guild and user.
func execSet(fx *cmdFixture, args ...string) error {
	cmd := fx.b.Registry.Get("set")
	Expect(cmd).NotTo(BeNil())

	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "200",
			GuildID:   "100",
			Author:    &discordgo.User{ID: "user1"},
			Content:   "e!set",
		},
	}
	return cmd.Exec(fx.sess.Session, m, args)
}

// makeNonAdminMember rewrites the fixture's member and guild so user1 holds no
// administrator permission.
func makeNonAdminMember(fx *cmdFixture) {
	m := &discordgo.Member{
		GuildID: "100",
		User:    &discordgo.User{ID: "user1", Username: "test"},
		Roles:   []string{},
	}
	_ = fx.sess.Session.State.MemberAdd(m)

	fx.sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
		g := &discordgo.Guild{
			ID:      "100",
			Name:    "Test Guild",
			OwnerID: "999",
			Roles:   []*discordgo.Role{{ID: "role1", Permissions: 0}},
		}
		return jsonMarshal(g), 200
	})
}

// captureChannelPosts registers a /channels/ handler that records the body of
// every POST so tests can assert on the embeds commands send.
func captureChannelPosts(fx *cmdFixture) *[]byte {
	captured := make([]byte, 0)
	fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
		if req.Method == http.MethodPost {
			captured = append(captured, readBody(req)...)
			return jsonMarshal(&discordgo.Message{ID: "r1"}), 200
		}
		return jsonMarshal(&discordgo.Channel{ID: "200", Name: "general", GuildID: "100"}), 200
	})
	return &captured
}

// assertSetCommandSuccess checks a captured POST body for the success template, the
// setting name, and the reported new value.
func assertSetCommandSuccess(captured *[]byte, setting, value string) error {
	// Go's encoding/json HTML-escapes <, > and &, so normalise first.
	body := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&").Replace(string(*captured))

	switch {
	case !strings.Contains(body, "Successfully changed setting"):
		return errors.New("missing success template")
	case !strings.Contains(body, setting):
		return fmt.Errorf("missing setting name %q", setting)
	case !strings.Contains(body, value):
		return fmt.Errorf("missing new value %q", value)
	}

	return nil
}

func readBody(req *http.Request) []byte {
	if req.Body == nil {
		return nil
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := req.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf
}
