package store

import (
	"context"
	"fmt"
	"os"
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
	Database *mongo.Database
	Guilds   *Guilds
	Messages *Messages
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	uri := cfg.URI
	if uri == "" {
		uri = os.Getenv("MONGODB_URL")
	}

	if uri == "" {
		return nil, fmt.Errorf("store: no MongoDB URI provided")
	}

	dbName := cfg.Database
	if dbName == "" {
		dbName = os.Getenv("EUG_DB_NAME")
	}

	if dbName == "" {
		dbName = DefaultDatabaseName
	}

	timeout := cfg.ConnectTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("store: connecting to MongoDB: %w", err)
	}

	if err := client.Ping(connectCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("store: pinging MongoDB (%s): %w", uri, err)
	}

	db := client.Database(dbName)

	return &Store{
		client:   client,
		Database: db,
		Guilds:   NewGuilds(db),
		Messages: NewMessages(db),
	}, nil
}

func (s *Store) Disconnect(ctx context.Context) error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Disconnect(ctx)
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("store: not connected")
	}

	return s.client.Ping(ctx, nil)
}
