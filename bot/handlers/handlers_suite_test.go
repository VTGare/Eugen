package handlers_test

import (
	"log/slog"
	"os"
	"testing"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/bot/registry"
	"github.com/VTGare/Eugen/bot/starboard"
	"github.com/VTGare/Eugen/store"
	"github.com/VTGare/Eugen/testutil"
	"github.com/bwmarrin/discordgo"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Handlers Suite")
}

var mongoContainer *testutil.MongoContainer

var _ = BeforeSuite(func() {
	mongoContainer = testutil.StartMongoDB(GinkgoT())
	DeferCleanup(func() {
		mongoContainer.Cleanup()
	})
})

type botFixture struct {
	b    *bot.Bot
	sess *testutil.MockSession
}

func newBotFixture() *botFixture {
	st := mongoContainer.NewTestStore(GinkgoT())
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	b := bot.New(st, bot.Config{Prefixes: []string{"e!"}}, logger)
	b.SetMention("<@123456789>")

	g := &registry.CommandGroup{
		Name:        "basic",
		Description: "test commands",
		IsVisible:   true,
		Commands: map[string]*registry.Command{
			"ping": {
				Name:        "ping",
				Description: "Check bot",
				Help:        &registry.HelpSettings{IsVisible: true},
				Exec: func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
					return nil
				},
			},
			"guildonly": {
				Name:        "guildonly",
				Description: "Guild-only command",
				GuildOnly:   true,
				Exec: func(s *discordgo.Session, m *discordgo.MessageCreate, args []string) error {
					return nil
				},
			},
		},
	}
	b.Registry.Add("basic", g)

	sess := testutil.NewSession(GinkgoT())
	sess.WithBotUser("123456789", "Eugen")
	b.Session = sess.Session

	sb := starboard.New(sess.Session, st, logger)
	b.SetStarboarder(sb)

	return &botFixture{b: b, sess: sess}
}

func (f *botFixture) addTestGuild(guildID, name string) {
	g := store.NewGuild(name, guildID)
	f.b.Store.Guilds.Cache().CacheSet(g)

	guild := &discordgo.Guild{
		ID:      guildID,
		Name:    name,
		OwnerID: "123456789",
		Roles: []*discordgo.Role{
			{ID: "@everyone", Permissions: discordgo.PermissionAdministrator},
		},
	}
	_ = f.sess.Session.State.GuildAdd(guild)
}
