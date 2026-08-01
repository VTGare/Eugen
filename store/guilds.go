package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const guildsCollection = "guilds"

// Emoji is the minimal interface over a Discord emoji that Guild.ValidateEmoji
// needs, so the store package doesn't import discordgo directly.
type Emoji interface {
	MessageFormat() string
}

type ChannelSettings struct {
	ID              string `json:"id" bson:"id"`
	StarRequirement int    `json:"star_requirement" bson:"star_requirement"`
}

type Guild struct {
	Prefix               string             `json:"prefix" bson:"prefix"`
	ID                   string             `json:"guild_id" bson:"guild_id"`
	Name                 string             `json:"name" bson:"name"`
	StarEmote            string             `json:"emote" bson:"emote"`
	EmbedColour          int64              `json:"color" bson:"color"`
	Enabled              bool               `json:"enabled" bson:"enabled"`
	StarboardChannel     string             `json:"starboard" bson:"starboard"`
	NSFWStarboardChannel string             `json:"nsfwstarboard" bson:"nsfwstarboard"`
	Selfstar             bool               `json:"selfstar" bson:"selfstar"`
	IgnoreBots           bool               `json:"ignorebots" bson:"ignorebots"`
	MinimumStars         int                `json:"stars" bson:"stars"`
	ChannelSettings      []*ChannelSettings `json:"channel_settings" bson:"channel_settings"`
	BlacklistedUsers     []string           `json:"blacklisted_users" bson:"blacklisted_users"`
	BannedChannels       []string           `json:"banned" bson:"banned"`
	CreatedAt            time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at" bson:"updated_at"`
}

// Guilds wraps the guilds collection and its in-memory cache.
type Guilds struct {
	col   *mongo.Collection
	cache *GuildCache
}

// GuildCache is the concurrency-safe in-memory cache for guild config.
type GuildCache struct {
	mu     sync.RWMutex
	guilds map[string]*Guild
}

// NewGuilds returns a Guilds bound to the given database. The cache starts
// empty — call LoadIntoCache at startup.
func NewGuilds(db *mongo.Database) *Guilds {
	return &Guilds{
		col:   db.Collection(guildsCollection),
		cache: &GuildCache{guilds: make(map[string]*Guild)},
	}
}

func (g *Guilds) Cache() *GuildCache { return g.cache }

func NewGuild(guildName, guildID string) *Guild {
	now := time.Now()
	return &Guild{
		Prefix:               "e!",
		ID:                   guildID,
		MinimumStars:         5,
		Name:                 guildName,
		StarEmote:            "⭐",
		Enabled:              true,
		Selfstar:             true,
		IgnoreBots:           false,
		EmbedColour:          4431601,
		StarboardChannel:     "",
		NSFWStarboardChannel: "",
		BlacklistedUsers:     make([]string, 0),
		ChannelSettings:      make([]*ChannelSettings, 0),
		BannedChannels:       make([]string, 0),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func (g *Guild) StarsRequired(channelID string) int {
	if idx := slices.IndexFunc(g.ChannelSettings, func(cs *ChannelSettings) bool {
		return cs.ID == channelID
	}); idx >= 0 {
		return g.ChannelSettings[idx].StarRequirement
	}
	return g.MinimumStars
}

func (g *Guild) IsBanned(channelID string) bool {
	return slices.Contains(g.BannedChannels, channelID)
}

func (g *Guild) ValidateEmoji(emoji Emoji) bool {
	return strings.EqualFold(g.StarEmote, emoji.MessageFormat())
}

func (g *Guild) IsGuildEmoji() bool {
	return strings.HasPrefix(g.StarEmote, "<:")
}

func (g *Guild) ChannelSettingsToString() string {
	var sb strings.Builder
	if len(g.ChannelSettings) == 0 {
		return "none"
	}
	sb.WriteString(fmt.Sprintf("<#%v>``%v``: %v ", g.ChannelSettings[0].ID, g.ChannelSettings[0].ID, g.ChannelSettings[0].StarRequirement))
	inRow := 1
	if len(g.ChannelSettings) > 1 {
		for _, ch := range g.ChannelSettings[1:] {
			if inRow == 2 {
				sb.WriteString(fmt.Sprintf("\n<#%v>``%v``: %v ", ch.ID, ch.ID, ch.StarRequirement))
				inRow = 0
			} else {
				sb.WriteString(fmt.Sprintf("| <#%v>``%v``: %v ", ch.ID, ch.ID, ch.StarRequirement))
			}
			inRow++
		}
	}
	return sb.String()
}

func (g *Guild) BannedChannelsToString() string {
	var sb strings.Builder
	if len(g.BannedChannels) == 0 {
		return "none"
	}
	sb.WriteString(fmt.Sprintf("<#%v>``%v`` ", g.BannedChannels[0], g.BannedChannels[0]))
	inRow := 1
	if len(g.BannedChannels) > 1 {
		for _, ch := range g.BannedChannels[1:] {
			if inRow == 2 {
				sb.WriteString(fmt.Sprintf("\n<#%v>``%v`` ", ch, ch))
				inRow = 0
			} else {
				sb.WriteString(fmt.Sprintf("| <#%v>``%v``", ch, ch))
			}
			inRow++
		}
	}
	return sb.String()
}

func (g *Guild) BlacklistedToString() string {
	var sb strings.Builder
	if len(g.BlacklistedUsers) == 0 {
		return "none"
	}
	fmt.Fprintf(&sb, "<@%v>", g.BlacklistedUsers[0])
	if len(g.BlacklistedUsers) > 1 {
		for _, user := range g.BlacklistedUsers[1:] {
			fmt.Fprintf(&sb, "| <@%v>", user)
		}
	}
	return sb.String()
}

// --- Cache methods ---

func (c *GuildCache) CacheSet(g *Guild) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.guilds[g.ID] = g
}

func (c *GuildCache) CacheDelete(guildID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.guilds, guildID)
}

func (c *GuildCache) Get(guildID string) *Guild {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.guilds[guildID]
}

// LoadIntoCache loads all guilds from the collection into the cache.
func (g *Guilds) LoadIntoCache(ctx context.Context) (int, error) {
	cur, err := g.col.Find(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("store: listing guilds for cache: %w", err)
	}
	defer cur.Close(ctx)

	guilds := make([]*Guild, 0)
	if err := cur.All(ctx, &guilds); err != nil {
		return 0, fmt.Errorf("store: decoding guilds for cache: %w", err)
	}

	g.cache.mu.Lock()
	defer g.cache.mu.Unlock()
	g.cache.guilds = make(map[string]*Guild, len(guilds))
	for _, guild := range guilds {
		g.cache.guilds[guild.ID] = guild
	}
	return len(guilds), nil
}

// --- CRUD operations ---

// All returns every guild document.
func (g *Guilds) All(ctx context.Context) ([]*Guild, error) {
	cur, err := g.col.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("store: listing guilds: %w", err)
	}
	defer cur.Close(ctx)

	guilds := make([]*Guild, 0)
	if err := cur.All(ctx, &guilds); err != nil {
		return nil, fmt.Errorf("store: decoding guilds: %w", err)
	}
	return guilds, nil
}

// Insert persists a single guild.
func (g *Guilds) Insert(ctx context.Context, guild *Guild) error {
	if _, err := g.col.InsertOne(ctx, guild); err != nil {
		return fmt.Errorf("store: inserting guild %s: %w", guild.ID, err)
	}
	g.cache.CacheSet(guild)
	return nil
}

// InsertMany persists a slice of guilds via bulk insert.
func (g *Guilds) InsertMany(ctx context.Context, guilds []*Guild) error {
	docs := make([]any, len(guilds))
	for i, g := range guilds {
		docs[i] = g
	}
	if _, err := g.col.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("store: inserting %d guilds: %w", len(guilds), err)
	}
	for _, guild := range guilds {
		g.cache.CacheSet(guild)
	}
	return nil
}

// Replace performs a full-document replacement by Guild.ID.
func (g *Guilds) Replace(ctx context.Context, guild *Guild) error {
	res := g.col.FindOneAndReplace(ctx, bson.M{"guild_id": guild.ID}, guild)
	if err := res.Err(); err != nil {
		return fmt.Errorf("store: replacing guild %s: %w", guild.ID, err)
	}
	updated := &Guild{}
	if err := res.Decode(updated); err != nil {
		g.cache.CacheSet(guild)
		return nil
	}
	g.cache.CacheSet(updated)
	return nil
}

func (g *Guilds) Delete(ctx context.Context, guildID string) error {
	if _, err := g.col.DeleteOne(ctx, bson.M{"guild_id": guildID}); err != nil {
		return fmt.Errorf("store: removing guild %s: %w", guildID, err)
	}
	g.cache.CacheDelete(guildID)
	return nil
}

// SetField sets a single field on a guild document and returns the updated doc.
func (g *Guilds) SetField(ctx context.Context, guildID, field string, value any) (*Guild, error) {
	res := g.col.FindOneAndUpdate(
		ctx,
		bson.M{"guild_id": guildID},
		bson.M{"$set": bson.M{field: value, "updated_at": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	guild := &Guild{}
	if err := res.Decode(guild); err != nil {
		return nil, fmt.Errorf("store: setting %s on guild %s: %w", field, guildID, err)
	}
	g.cache.CacheSet(guild)
	return guild, nil
}

// --- Guild mutation operations ---

func (g *Guilds) BanChannel(ctx context.Context, guildID, channelID string) error {
	return g.cacheAndReturn(ctx, guildID, bson.M{"$addToSet": bson.M{"banned": channelID}})
}

func (g *Guilds) UnbanChannel(ctx context.Context, guildID, channelID string) error {
	return g.cacheAndReturn(ctx, guildID, bson.M{"$pull": bson.M{"banned": channelID}})
}

func (g *Guilds) BanUser(ctx context.Context, guildID, userID string) error {
	return g.cacheAndReturn(ctx, guildID, bson.M{"$addToSet": bson.M{"blacklisted_users": userID}})
}

func (g *Guilds) UnbanUser(ctx context.Context, guildID, userID string) error {
	return g.cacheAndReturn(ctx, guildID, bson.M{"$pull": bson.M{"blacklisted_users": userID}})
}

// SetChannelStars sets a per-channel star requirement. If the channel already
// has an override, it's updated via the positional operator; otherwise a new
// entry is added.
func (g *Guilds) SetChannelStars(ctx context.Context, guildID, channelID string, stars int) error {
	cs := &ChannelSettings{ID: channelID, StarRequirement: stars}
	now := time.Now()

	res := g.col.FindOneAndUpdate(
		ctx,
		bson.M{"guild_id": guildID, "channel_settings.id": channelID},
		bson.M{"$set": bson.M{"updated_at": now, "channel_settings.$.star_requirement": stars}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)

	guild := &Guild{}
	if err := res.Decode(guild); err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("store: setting channel stars for %s in guild %s: %w", channelID, guildID, err)
		}
		// Channel not configured — add it.
		res = g.col.FindOneAndUpdate(
			ctx,
			bson.M{"guild_id": guildID},
			bson.M{"$set": bson.M{"updated_at": now}, "$addToSet": bson.M{"channel_settings": cs}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		)
		guild = &Guild{}
		if err := res.Decode(guild); err != nil {
			return fmt.Errorf("store: adding channel stars for %s in guild %s: %w", channelID, guildID, err)
		}
	}

	g.cache.CacheSet(guild)
	return nil
}

func (g *Guilds) UnsetChannelStars(ctx context.Context, guildID, channelID string) error {
	return g.cacheAndReturn(ctx, guildID, bson.M{"$pull": bson.M{"channel_settings": bson.M{"id": channelID}}})
}

// CreateIndex creates recommended indexes. Idempotent.
func (g *Guilds) CreateIndex(ctx context.Context) error {
	_, err := g.col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "guild_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("store: creating guilds unique index: %w", err)
	}
	return nil
}

// cacheAndReturn runs a FindOneAndUpdate with the given mutation (merging
// updated_at), returns the post-update guild, and caches it.
func (g *Guilds) cacheAndReturn(ctx context.Context, guildID string, mut bson.M) error {
	set, ok := mut["$set"].(bson.M)
	if !ok {
		set = bson.M{}
	}
	if _, exists := set["updated_at"]; !exists {
		set["updated_at"] = time.Now()
	}
	mut["$set"] = set

	res := g.col.FindOneAndUpdate(
		ctx,
		bson.M{"guild_id": guildID},
		mut,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	guild := &Guild{}
	if err := res.Decode(guild); err != nil {
		return fmt.Errorf("store: mutating guild %s: %w", guildID, err)
	}
	g.cache.CacheSet(guild)
	return nil
}
