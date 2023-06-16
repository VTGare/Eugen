package bot

import (
	"context"
	"fmt"

	"github.com/VTGare/Eugen/store"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/api/cmdroute"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/state"
	"github.com/knadh/koanf/v2"
	"go.uber.org/zap"
)

type Bot struct {
	Config *koanf.Koanf
	State  *state.State
	Store  store.Store
	Log    *zap.SugaredLogger

	router   *cmdroute.Router
	commands []api.CreateCommandData
}

func New(log *zap.SugaredLogger, config *koanf.Koanf) *Bot {
	var (
		r = cmdroute.NewRouter()
		s = state.New("Bot " + config.String("bot.token"))
	)

	s.AddIntents(gateway.IntentGuildEmojis |
		gateway.IntentGuildMessageReactions |
		gateway.IntentGuildMessages |
		gateway.IntentMessageContent |
		gateway.IntentDirectMessages,
	)

	return &Bot{
		Config: config,
		State:  s,
		Log:    log,

		router:   r,
		commands: make([]api.CreateCommandData, 0),
	}
}

func (b *Bot) AddCommand(f func(b *Bot) (command api.CreateCommandData, handler cmdroute.CommandHandlerFunc)) {
	cmd, handler := f(b)

	b.commands = append(b.commands, cmd)
	b.router.AddFunc(cmd.Name, handler)
}

func (b *Bot) AddMiddleware(mw cmdroute.Middleware) {
	b.router.Use(mw)
}

func (b *Bot) Start(ctx context.Context) error {
	b.State.AddInteractionHandler(b.router)

	if err := cmdroute.OverwriteCommands(b.State, b.commands); err != nil {
		return fmt.Errorf("failed to overwrite commands: %w", err)
	}

	if err := b.State.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	return nil
}
