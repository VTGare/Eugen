package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/bot/commands"
	"github.com/VTGare/Eugen/bot/handlers"
	"github.com/VTGare/Eugen/config"
	"github.com/VTGare/Eugen/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "err", err)
		os.Exit(1)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)

	st := setupStore(cfg, log)

	b := bot.New(st, bot.Config{
		Prefixes: cfg.Bot.Prefixes,
	}, log)

	// Register commands into the bot's registry.
	commands.Register(b)

	// Create and connect the Discord session.
	if err := b.Start(cfg.Bot.Token); err != nil {
		log.Error("creating session", "err", err)
		os.Exit(1)
	}

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
	defer s.Close()

	// Create database indexes.
	if err := b.CreateIndexes(context.Background()); err != nil {
		log.Warn("creating indexes", "err", err)
	}

	// Seed guild cache from database.
	b.LoadGuildCache()

	// Wait for interrupt signal.
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	<-sc

	if err := st.Disconnect(context.Background()); err != nil {
		log.Warn("disconnecting from mongodb", "err", err)
	}
}

func setupStore(cfg *config.Config, log *slog.Logger) *store.Store {
	s, err := store.New(context.Background(), store.Config{
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
