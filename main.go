package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/VTGare/Eugen/arikawautils/middlewares"
	"github.com/VTGare/Eugen/bot"
	"github.com/VTGare/Eugen/commands"
	"github.com/VTGare/Eugen/ctxzap"

	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"go.uber.org/zap"
)

var config = koanf.NewWithConf(koanf.Conf{
	Delim:       ".",
	StrictMerge: true,
})

func main() {
	if err := initializeConfig(); err != nil {
		log.Fatalf("failed to intialize config: %v", err)
	}

	log, err := initializeLogger()
	if err != nil {
		log.Fatal("failed to initialize logger: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx = ctxzap.ToContext(ctx, log)

	b := bot.New(log, config)

	b.AddMiddleware(middlewares.CommandLog(log))
	commands.RegisterCommands(b)

	if err := b.Start(ctx); err != nil {
		log.With("error", err).Fatal("failed to start the bot")
	}
}

func initializeLogger() (*zap.SugaredLogger, error) {
	if config.Bool("dev.mode") {
		log, err := zap.NewDevelopment()
		if err != nil {
			return nil, err
		}

		return log.Sugar(), nil
	}

	log, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}

	return log.Sugar(), nil
}

func initializeConfig() error {
	// Load JSON config
	jsonPath := "config.json"
	if fileExists(jsonPath) {
		if err := config.Load(file.Provider(jsonPath), json.Parser()); err != nil {
			return err
		}
	}

	// Load environment variables
	err := config.Load(env.Provider("EUGEN_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "EUGEN_")), "_", ".", -1)
	}), nil)
	if err != nil {
		return err
	}

	// Load .env file
	dotenvPath := ".env"
	if fileExists(dotenvPath) {
		dotenvParser := dotenv.ParserEnv("EUGEN_", ".", func(s string) string {
			return strings.Replace(strings.ToLower(
				strings.TrimPrefix(s, "EUGEN_")), "_", ".", -1)
		})

		if err := config.Load(file.Provider(".env"), dotenvParser); err != nil {
			return err
		}
	}

	return nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}
