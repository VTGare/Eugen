package commands_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/Eugen/utils"
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
		Expect(err).To(MatchError(utils.ErrNotEnoughArguments))
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
		Expect(err).To(MatchError(utils.ErrNotEnoughArguments))
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
		Expect(err).To(MatchError(utils.ErrNotEnoughArguments))
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
		Expect(err).To(MatchError(utils.ErrNotEnoughArguments))
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
		Expect(err).To(MatchError(utils.ErrNotEnoughArguments))
	})
})

var _ = Describe("set command", func() {
	var fx *cmdFixture

	BeforeEach(func() {
		fx = newCmdFixture()
		fx.addTestGuild("100", "Test Guild")
		fx.addTestMember("100", "user1")
	})

	It("shows guild settings when no args", func() {
		var capturedBody []byte
		fx.sess.On("/channels/", func(req *http.Request) ([]byte, int) {
			if req.Method == "POST" {
				capturedBody = readBody(req)
			}
			return jsonMarshal(&discordgo.Message{ID: "r1"}), 200
		})

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
		err := cmd.Exec(fx.sess.Session, m, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(capturedBody)).To(ContainSubstring("Current settings"))
	})

	It("returns incorrect usage for 1 arg", func() {
		cmd := fx.b.Registry.Get("set")
		Expect(cmd).NotTo(BeNil())

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "200",
				GuildID:   "100",
				Author:    &discordgo.User{ID: "user1"},
			},
		}
		err := cmd.Exec(fx.sess.Session, m, []string{"onlyone"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("incorrect command usage"))
	})
})

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
