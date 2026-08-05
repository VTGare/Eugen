package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	messagesCollection = "messages"
	MessageCacheTTL    = 10 * time.Hour
)

type MessagePair struct {
	ChannelID string `bson:"channel_id" json:"channel_id"`
	MessageID string `bson:"message_id" json:"message_id"`
}

func (p MessagePair) String() string {
	return p.ChannelID + " " + p.MessageID
}

type Message struct {
	ID        bson.ObjectID `bson:"_id,omitempty" json:"-"`
	GuildID   string        `bson:"guild_id" json:"guild_id"`
	Original  *MessagePair  `bson:"original" json:"original"`
	Starboard *MessagePair  `bson:"starboard" json:"starboard"`
	CreatedAt time.Time     `bson:"created_at" json:"created_at"`
}

func NewMessage(original, starboard *MessagePair, guildID string) *Message {
	return &Message{GuildID: guildID, Original: original, Starboard: starboard, CreatedAt: time.Now()}
}

func NewPair(channelID, messageID string) MessagePair {
	return MessagePair{ChannelID: channelID, MessageID: messageID}
}

// Messages wraps the messages collection with an in-memory cache.
type Messages struct {
	col   *mongo.Collection
	cache *messageCache
}

func NewMessages(db *mongo.Database) *Messages {
	m := &Messages{
		col:   db.Collection(messagesCollection),
		cache: newMessageCache(MessageCacheTTL),
	}
	m.cache.start()
	return m
}

type messageEntry struct {
	msg    Message
	expiry time.Time
}

type messageCache struct {
	mu   sync.RWMutex
	ttl  time.Duration
	data map[MessagePair]messageEntry
}

func newMessageCache(ttl time.Duration) *messageCache {
	return &messageCache{ttl: ttl, data: make(map[MessagePair]messageEntry)}
}

func (c *messageCache) start() {
	ticker := time.NewTicker(c.ttl / 2)
	go func() {
		for range ticker.C {
			c.evictExpired()
		}
	}()
}

func (c *messageCache) get(pair MessagePair) (Message, bool) {
	c.mu.RLock()
	e, ok := c.data[pair]
	c.mu.RUnlock()

	if !ok {
		return Message{}, false
	}

	if time.Now().After(e.expiry) {
		c.mu.Lock()
		delete(c.data, pair)
		c.mu.Unlock()
		return Message{}, false
	}

	return e.msg, true
}

func (c *messageCache) set(pair MessagePair, msg Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[pair] = messageEntry{msg: msg, expiry: time.Now().Add(c.ttl)}
}

func (c *messageCache) delete(pair MessagePair) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, pair)
}

func (c *messageCache) evictExpired() {
	now := time.Now()
	c.mu.Lock()
	for k, e := range c.data {
		if now.After(e.expiry) {
			delete(c.data, k)
		}
	}
	c.mu.Unlock()
}

// Insert persists a single message record.
func (m *Messages) Insert(ctx context.Context, msg *Message) error {
	if _, err := m.col.InsertOne(ctx, msg); err != nil {
		return fmt.Errorf("store: inserting message (%s/%s): %w", msg.Original.ChannelID, msg.Original.MessageID, err)
	}
	m.cache.set(*msg.Original, *msg)
	return nil
}

// InsertMany persists a batch of message records.
func (m *Messages) InsertMany(ctx context.Context, docs []any) error {
	if len(docs) == 0 {
		return nil
	}
	if _, err := m.col.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("store: inserting %d messages: %w", len(docs), err)
	}
	for _, d := range docs {
		switch msg := d.(type) {
		case *Message:
			m.cache.set(*msg.Original, *msg)
		case Message:
			m.cache.set(*msg.Original, msg)
		default:
			slog.Warn("skipping cache for non-message doc", "type", fmt.Sprintf("%T", d))
		}
	}
	return nil
}

func (m *Messages) Delete(ctx context.Context, pair *MessagePair) error {
	filter := bson.D{
		{Key: "original.channel_id", Value: pair.ChannelID},
		{Key: "original.message_id", Value: pair.MessageID},
	}
	if _, err := m.col.DeleteOne(ctx, filter); err != nil {
		return fmt.Errorf("store: deleting message (%s/%s): %w", pair.ChannelID, pair.MessageID, err)
	}
	m.cache.delete(*pair)
	return nil
}

// Repost returns the starboard record for an original message.
func (m *Messages) Repost(ctx context.Context, channelID, id string) (*Message, error) {
	pair := NewPair(channelID, id)

	if msg, ok := m.cache.get(pair); ok {
		return &msg, nil
	}

	msg := &Message{}
	err := m.col.FindOne(ctx, bson.D{
		{Key: "original.channel_id", Value: channelID},
		{Key: "original.message_id", Value: id},
	}).Decode(msg)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: finding repost (%s/%s): %w", channelID, id, err)
	}
	m.cache.set(pair, *msg)
	return msg, nil
}

// RepostByStarboard returns the starboard record for a starboard message.
func (m *Messages) RepostByStarboard(ctx context.Context, channelID, id string) (*Message, error) {
	msg := &Message{}
	err := m.col.FindOne(ctx, bson.D{
		{Key: "starboard.channel_id", Value: channelID},
		{Key: "starboard.message_id", Value: id},
	}).Decode(msg)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: finding repost by starboard (%s/%s): %w", channelID, id, err)
	}
	return msg, nil
}

func (m *Messages) CreateIndex(ctx context.Context) error {
	_, err := m.col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "original.channel_id", Value: 1},
			{Key: "original.message_id", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("store: creating messages unique index: %w", err)
	}

	return nil
}
