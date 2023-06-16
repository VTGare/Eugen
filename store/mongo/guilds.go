package mongo

import (
	"context"
	"time"

	"github.com/VTGare/Eugen/ctxzap"
	"github.com/VTGare/Eugen/store"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type guildStore struct {
	client *mongo.Client
	db     *mongo.Database
	col    *mongo.Collection
}

func (gs *guildStore) Guild(ctx context.Context, guildID string) (*store.Guild, error) {
	log := ctxzap.Extract(ctx)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	res := gs.col.FindOne(ctx, bson.M{"guild_id": guildID})

	var guild store.Guild
	err := res.Decode(&guild)
	if err != nil {
		log.With("guild_id", guildID, "error", err).
			Error("failed to decode a guild")

		return nil, handleGuildError(err)
	}

	return &guild, nil
}

func (gs *guildStore) CreateGuild(ctx context.Context, guildID string) (*store.Guild, error) {
	log := ctxzap.Extract(ctx)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	guild := store.DefaultGuild(guildID)
	_, err := gs.col.InsertOne(ctx, guild)
	if err != nil {
		log.With("guild_id", guildID, "error", err).
			Error("failed to insert a guild")
		return nil, handleGuildError(err)
	}

	return guild, nil
}

func (gs *guildStore) UpdateGuild(ctx context.Context, guild *store.Guild) (*store.Guild, error) {
	log := ctxzap.Extract(ctx)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	_, err := gs.col.ReplaceOne(ctx, bson.M{"guild_id": guild.ID}, guild, options.Replace().SetUpsert(false))
	if err != nil {
		log.With("guild", guild, "error", err).
			Error("failed to replace a guild")
		return nil, handleGuildError(err)
	}

	return guild, nil
}

func (gs *guildStore) DeleteGuild(ctx context.Context, guildID string) error {
	log := ctxzap.Extract(ctx)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	_, err := gs.col.DeleteOne(ctx, bson.M{"guild_id": guildID})
	if err != nil {
		log.With("guild_id", guildID, "error", err).
			Error("failed to replace a guild")
		return handleGuildError(err)
	}

	return nil
}

func handleGuildError(err error) error {
	switch err {
	case mongo.ErrNoDocuments:
		return store.ErrGuildNotFound
	default:
		return store.ErrInternal
	}
}
