package mongo

import (
	"context"
	"time"

	"github.com/VTGare/Eugen/ctxzap"
	"github.com/VTGare/Eugen/store"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type messageStore struct {
	client *mongo.Client
	db     *mongo.Database
	col    *mongo.Collection
}

func (ms *messageStore) Message(ctx context.Context, channelID, messageID string) (*store.Message, error) {
	log := ctxzap.Extract(ctx)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	filter := bson.M{
		"original.channel_id": channelID,
		"original.message_id": messageID,
	}

	res := ms.col.FindOne(ctx, filter)

	var message store.Message
	err := res.Decode(&message)
	if err != nil {
		log.With("channel_id", channelID, "message_id", messageID).
			Error("failed to decode message")
		return nil, handleMessageError(err)
	}

	return &message, nil
}

func (ms *messageStore) MessageByStarboardReference(ctx context.Context, channelID, messageID string) (*store.Message, error) {
	log := ctxzap.Extract(ctx)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	filter := bson.M{
		"starboard.channel_id": channelID,
		"starboard.message_id": messageID,
	}

	res := ms.col.FindOne(ctx, filter)

	var message store.Message
	err := res.Decode(&message)
	if err != nil {
		log.With("channel_id", channelID, "message_id", messageID).
			Error("failed to decode a message")
		return nil, handleMessageError(err)
	}

	return &message, nil
}

func (ms *messageStore) CreateMessage(ctx context.Context, msg *store.Message) error {
	log := ctxzap.Extract(ctx)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	_, err := ms.col.InsertOne(ctx, msg)
	if err != nil {
		log.With("message", msg, "error", err).
			Error("failed to insert a message")
		return handleMessageError(err)
	}

	return nil
}

func (ms *messageStore) DeleteMessage(ctx context.Context, channelID, messageID string) error {
	log := ctxzap.Extract(ctx)

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	filter := bson.M{
		"$or": []bson.M{
			{
				"original.channel_id": channelID,
				"original.message_id": messageID,
			},
			{
				"starboard.channel_id": channelID,
				"starboard.message_id": messageID,
			},
		},
	}

	_, err := ms.col.DeleteOne(ctx, filter)
	if err != nil {
		log.With("channel_id", channelID, "message_id", messageID, "error", err).
			Error("failed to delete a message")
		return handleMessageError(err)
	}

	return nil
}

func handleMessageError(err error) error {
	switch err {
	case mongo.ErrNoDocuments:
		return store.ErrMessageNotFound
	default:
		return store.ErrInternal
	}
}
