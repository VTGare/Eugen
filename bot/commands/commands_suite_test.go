package commands_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/bot/commands"
	"github.com/VTGare/Eugen/store"
	"github.com/VTGare/Eugen/testutil"
	"github.com/bwmarrin/discordgo"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var mongoContainer *testutil.MongoContainer

func TestCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Commands Suite")
}

var _ = BeforeSuite(func() {
	mongoContainer = testutil.StartMongoDB(GinkgoT())
	DeferCleanup(func() {
		mongoContainer.Cleanup()
	})
})

type cmdFixture struct {
	b    *bot.Bot
	sess *testutil.MockSession
}

func newCmdFixture() *cmdFixture {
	st := mongoContainer.NewTestStore(GinkgoT())
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	b := bot.New(st, bot.Config{Prefixes: []string{"e!"}}, logger)
	b.SetMention("<@123456789>")
	commands.Register(b)

	sess := testutil.NewSession(GinkgoT())
	sess.WithBotUser("123456789", "Eugen")
	b.Session = sess.Session

	// Register mock REST handlers.
	sess.On("/guilds/", func(req *http.Request) ([]byte, int) {
		// GET /guilds/{guildID} → return guild with roles
		if req.Method == "GET" {
			g := &discordgo.Guild{
				ID:      "100",
				Name:    "Test Guild",
				OwnerID: "123456789",
				Roles: []*discordgo.Role{
					{ID: "@everyone", Permissions: discordgo.PermissionAdministrator},
				},
			}
			return jsonMarshal(g), 200
		}
		return nil, 404
	})

	sess.On("/channels/", func(req *http.Request) ([]byte, int) {
		if req.Method == "GET" {
			ch := &discordgo.Channel{
				ID:      "200",
				Name:    "general",
				GuildID: "100",
				Type:    discordgo.ChannelTypeGuildText,
			}
			return jsonMarshal(ch), 200
		}
		if req.Method == "POST" {
			m := &discordgo.Message{ID: "300", ChannelID: "200", Content: "ok"}
			return jsonMarshal(m), 200
		}
		return nil, 404
	})

	sess.On("/guilds/100/emojis", func(req *http.Request) ([]byte, int) {
		emojis := []*discordgo.Emoji{
			{ID: "999", Name: "star", Animated: false},
		}
		return jsonMarshal(emojis), 200
	})

	sess.On("/guilds/100/members/", func(req *http.Request) ([]byte, int) {
		member := &discordgo.Member{
			GuildID: "100",
			User:    &discordgo.User{ID: "user1", Username: "test"},
			Roles:   []string{},
		}
		return jsonMarshal(member), 200
	})

	b.Session = sess.Session
	return &cmdFixture{b: b, sess: sess}
}

func jsonMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// addTestGuildCache adds a guild to the store cache and the State cache.
func (f *cmdFixture) addTestGuild(guildID, name string) *store.Guild {
	g := store.NewGuild(name, guildID)
	f.b.Store.Guilds.Cache().CacheSet(g)

	dg := &discordgo.Guild{
		ID:      guildID,
		Name:    name,
		OwnerID: "123456789",
		Roles: []*discordgo.Role{
			{ID: "@everyone", Permissions: discordgo.PermissionAdministrator},
		},
	}
	_ = f.sess.Session.State.GuildAdd(dg)
	return g
}

// addTestMember adds a member to the state cache.
func (f *cmdFixture) addTestMember(guildID, userID string) {
	m := &discordgo.Member{
		GuildID: guildID,
		User:    &discordgo.User{ID: userID, Username: "testuser"},
		Roles:   []string{"@everyone"},
	}
	_ = f.sess.Session.State.MemberAdd(m)
}
