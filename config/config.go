// Package config handles all application configuration for Eugen.
// It reads from environment variables (with sensible defaults) or
// from a JSON configuration file, producing a single Config struct
// that is threaded through the application.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config is the top-level configuration for Eugen.
type Config struct {
	Bot     BotConfig     `json:"bot"`
	MongoDB MongoDBConfig `json:"mongodb"`
}

// BotConfig holds Discord bot settings.
type BotConfig struct {
	Token    string   `json:"token"`
	Prefixes []string `json:"prefixes"`
}

// MongoDBConfig holds MongoDB connection settings.
type MongoDBConfig struct {
	URI            string        `json:"uri"`
	Database       string        `json:"database"`
	ConnectTimeout time.Duration `json:"connect_timeout"`
}

const (
	defaultDatabase = "eugen"
	defaultTimeout  = 10 * time.Second
)

func defaults() Config {
	return Config{
		Bot: BotConfig{
			Prefixes: []string{"e!", "e.", "e "},
		},
		MongoDB: MongoDBConfig{
			Database:       defaultDatabase,
			ConnectTimeout: defaultTimeout,
		},
	}
}

// Load reads configuration from environment variables and an optional JSON file.
// The file path is set via the EUGEN_CONFIG environment variable.
// Environment variables override file configuration.
func Load() (*Config, error) {
	cfg := defaults()

	// Try loading from JSON config file.
	if path := os.Getenv("EUGEN_CONFIG"); path != "" {
		if err := loadFile(path, &cfg); err != nil {
			return nil, fmt.Errorf("config: loading JSON config: %w", err)
		}
	}

	// Environment variables take precedence over file config.
	if v := os.Getenv("EUGEN_BOT_TOKEN"); v != "" {
		cfg.Bot.Token = v
	}
	if v := os.Getenv("EUGEN_MONGODB_URL"); v != "" {
		cfg.MongoDB.URI = v
	}
	if v := os.Getenv("EUGEN_DB_NAME"); v != "" {
		cfg.MongoDB.Database = v
	}
	if v := os.Getenv("EUGEN_DB_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("config: parsing EUGEN_DB_TIMEOUT: %w", err)
		}
		cfg.MongoDB.ConnectTimeout = d
	}
	if v := os.Getenv("EUGEN_PREFIXES"); v != "" {
		cfg.Bot.Prefixes = strings.Split(v, ",")
	}

	// Validate required fields.
	if cfg.Bot.Token == "" {
		return nil, fmt.Errorf("config: EUGEN_BOT_TOKEN environment variable is required")
	}
	if cfg.MongoDB.URI == "" {
		return nil, fmt.Errorf("config: EUGEN_MONGODB_URL environment variable is required")
	}
	if cfg.MongoDB.Database == "" {
		cfg.MongoDB.Database = defaultDatabase
	}
	if cfg.MongoDB.ConnectTimeout == 0 {
		cfg.MongoDB.ConnectTimeout = defaultTimeout
	}

	return &cfg, nil
}

func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, cfg)
}
