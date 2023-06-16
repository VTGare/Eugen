package store

import (
	"context"
	"time"
)

type MessageStore interface {
	Message(ctx context.Context, channelID, messageID string) (*Message, error)
	MessageByStarboardReference(ctx context.Context, channelID, messageID string) (*Message, error)
	CreateMessage(ctx context.Context, msg *Message) error
	DeleteMessage(ctx context.Context, channelID, messageID string) error
}

type Message struct {
	GuildID   string           `bson:"guild_id" json:"guild_id"`
	Original  *MessageMetadata `bson:"original" json:"original"`
	Starboard *MessageMetadata `bson:"starboard" json:"starboard"`
	CreatedAt time.Time        `bson:"created_at" json:"created_at"`
}

type MessageMetadata struct {
	ChannelID string `bson:"channel_id" json:"channel_id"`
	MessageID string `bson:"message_id" json:"message_id"`
}
