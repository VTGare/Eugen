package store

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const DefaultDatabaseName = "eugen"

type Config struct {
	URI            string
	Database       string
	ConnectTimeout time.Duration
}

type Store struct {
	client   *mongo.Client
	database *mongo.Database
	Guilds   *Guilds
	Messages *Messages
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.URI == "" {
		return nil, fmt.Errorf("store: no MongoDB URI provided")
	}

	dbName := cfg.Database
	if dbName == "" {
		dbName = DefaultDatabaseName
	}

	timeout := cfg.ConnectTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("store: connecting to MongoDB: %w", err)
	}

	if err := client.Ping(connectCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("store: pinging MongoDB (%s): %w", cfg.URI, err)
	}

	db := client.Database(dbName)

	st := &Store{
		client:   client,
		database: db,
		Guilds:   newGuilds(db),
		Messages: newMessages(db),
	}

	indexCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := st.createIndexes(indexCtx); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	return st, nil
}

func (s *Store) createIndexes(ctx context.Context) error {
	if err := s.Guilds.createIndex(ctx); err != nil {
		return err
	}
	if err := s.Messages.createIndex(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) Disconnect(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Disconnect(ctx)
}
