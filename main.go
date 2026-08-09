package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/bot/commands"
	"github.com/VTGare/Eugen/bot/handlers"
	"github.com/VTGare/Eugen/bot/starboard"
	"github.com/VTGare/Eugen/config"
	"github.com/VTGare/Eugen/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "err", err)
		os.Exit(1)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)

	st := setupStore(ctx, cfg, log)

	b := bot.New(ctx, st, bot.Config{
		Prefixes: cfg.Bot.Prefixes,
	}, log)

	// Register commands into the bot's registry.
	commands.Register(b)

	// Create and connect the Discord session.
	if err := b.Start(cfg.Bot.Token); err != nil {
		log.Error("creating session", "err", err)
		os.Exit(1)
	}

	// Create and inject the starboard engine after the session is ready.
	b.SetStarboarder(starboard.New(ctx, b.Session, st, log))

	// Register event handlers.
	s := b.Session
	s.AddHandler(handlers.Ready(b))
	s.AddHandler(handlers.MessageCreate(b))
	s.AddHandler(handlers.GuildCreate(b))
	s.AddHandler(handlers.MessageReactionAdd(b))
	s.AddHandler(handlers.MessageReactionRemove(b))
	s.AddHandler(handlers.MessageReactionRemoveAll(b))
	s.AddHandler(handlers.MessageDelete(b))

	if err := s.Open(); err != nil {
		log.Error("opening connection", "err", err)
		os.Exit(1)
	}

	// Create database indexes.
	if err := b.CreateIndexes(); err != nil {
		log.Warn("creating indexes", "err", err)
	}

	// Seed guild cache from database.
	b.LoadGuildCache()

	// Block until a shutdown signal cancels the app context.
	<-ctx.Done()
	log.Info("shutdown signal received, cleaning up")

	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := b.Shutdown(cleanupCtx); err != nil {
		log.Warn("cleaning up", "err", err)
	}
}

func setupStore(ctx context.Context, cfg *config.Config, log *slog.Logger) *store.Store {
	s, err := store.New(ctx, store.Config{
		URI:            cfg.MongoDB.URI,
		Database:       cfg.MongoDB.Database,
		ConnectTimeout: cfg.MongoDB.ConnectTimeout,
	})
	if err != nil {
		log.Error("creating store", "err", err)
		os.Exit(1)
	}
	return s
}
