package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	Prefix           string             `json:"prefix" bson:"prefix"`
	ID               string             `json:"guild_id" bson:"guild_id"`
	Name             string             `json:"name" bson:"name"`
	StarEmote        string             `json:"emote" bson:"emote"`
	EmbedColor       int64              `json:"color" bson:"color"`
	Enabled          bool               `json:"enabled" bson:"enabled"`
	StarboardChannel string             `json:"starboard" bson:"starboard"`
	Selfstar         bool               `json:"selfstar" bson:"selfstar"`
	IgnoreBots       bool               `json:"ignorebots" bson:"ignorebots"`
	MinimumStars     int                `json:"stars" bson:"stars"`
	ChannelSettings  []*ChannelSettings `json:"channel_settings" bson:"channel_settings"`
	BlacklistedUsers []string           `json:"blacklisted_users" bson:"blacklisted_users"`
	BannedChannels   []string           `json:"banned" bson:"banned"`
	CreatedAt        time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" bson:"updated_at"`
}

// GuildPatch carries optional field updates for Guilds.Update. Non-nil fields
// are written to the stored guild; nil fields are left untouched.
type GuildPatch struct {
	Prefix           *string
	StarEmote        *string
	EmbedColor       *int64
	Enabled          *bool
	StarboardChannel *string
	Selfstar         *bool
	IgnoreBots       *bool
	MinimumStars     *int
}

// Guilds wraps the guilds collection and its in-memory cache.
type Guilds struct {
	col   *mongo.Collection
	cache *guildCache
}

// guildCache is the concurrency-safe in-memory cache for guild config.
type guildCache struct {
	mu     sync.RWMutex
	guilds map[string]*Guild
}

func newGuilds(db *mongo.Database) *Guilds {
	return &Guilds{
		col:   db.Collection(guildsCollection),
		cache: &guildCache{guilds: make(map[string]*Guild)},
	}
}

func NewGuild(guildName, guildID string) *Guild {
	now := time.Now()
	return &Guild{
		Prefix:           "e!",
		ID:               guildID,
		MinimumStars:     5,
		Name:             guildName,
		StarEmote:        "⭐",
		Enabled:          true,
		Selfstar:         true,
		IgnoreBots:       false,
		EmbedColor:       4431601,
		StarboardChannel: "",
		BlacklistedUsers: make([]string, 0),
		ChannelSettings:  make([]*ChannelSettings, 0),
		BannedChannels:   make([]string, 0),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// Get returns the guild config for guildID, reading from the cache and
// falling back to the database on a cache miss. Returns nil if the guild is
// unknown; transient database errors are logged and also yield nil. The caller
// controls the timeout through the provided context.
func (g *Guilds) Get(ctx context.Context, guildID string) *Guild {
	if cached := g.cache.get(guildID); cached != nil {
		return cached
	}

	guild := &Guild{}
	err := g.col.FindOne(ctx, bson.M{"guild_id": guildID}).Decode(guild)
	if err != nil {
		if !errors.Is(err, mongo.ErrNoDocuments) {
			slog.Warn("store: read-through guild lookup failed", "guild_id", guildID, "err", err)
		}
		return nil
	}

	g.cache.set(guild)
	return guild
}

// List returns every guild document from the collection. It does not touch
// the cache.
func (g *Guilds) List(ctx context.Context) ([]*Guild, error) {
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

// Refresh rebuilds the cache from the collection and returns how many guilds
// were loaded. Call this at startup to warm the cache before the first event.
func (g *Guilds) Refresh(ctx context.Context) (int, error) {
	guilds, err := g.List(ctx)
	if err != nil {
		return 0, err
	}

	g.cache.mu.Lock()
	defer g.cache.mu.Unlock()

	g.cache.guilds = make(map[string]*Guild, len(guilds))
	for _, guild := range guilds {
		g.cache.guilds[guild.ID] = guild
	}

	return len(guilds), nil
}

// Create persists a single guild document and caches it.
func (g *Guilds) Create(ctx context.Context, guild *Guild) error {
	if _, err := g.col.InsertOne(ctx, guild); err != nil {
		return fmt.Errorf("store: inserting guild %s: %w", guild.ID, err)
	}

	g.cache.set(guild)
	return nil
}

// CreateMany persists a slice of guilds via bulk insert and caches them all.
func (g *Guilds) CreateMany(ctx context.Context, guilds []*Guild) error {
	if len(guilds) == 0 {
		return nil
	}

	docs := make([]any, len(guilds))
	for i, guild := range guilds {
		docs[i] = guild
	}

	if _, err := g.col.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("store: inserting %d guilds: %w", len(guilds), err)
	}

	for _, guild := range guilds {
		g.cache.set(guild)
	}
	return nil
}

// Update writes the non-nil fields of patch to the guild and refreshes its
// cached copy.
func (g *Guilds) Update(ctx context.Context, guildID string, patch GuildPatch) error {
	set := bson.M{"updated_at": time.Now()}

	if patch.Prefix != nil {
		if len(*patch.Prefix) > 5 {
			return errors.New("prefix is too long, max 5 characters")
		}
		set["prefix"] = *patch.Prefix
	}
	if patch.StarEmote != nil {
		set["emote"] = *patch.StarEmote
	}
	if patch.EmbedColor != nil {
		if *patch.EmbedColor < 0 || *patch.EmbedColor > 16777215 {
			return errors.New("color must be in range 0 to 16777215")
		}
		set["color"] = *patch.EmbedColor
	}
	if patch.Enabled != nil {
		set["enabled"] = *patch.Enabled
	}
	if patch.StarboardChannel != nil {
		set["starboard"] = *patch.StarboardChannel
	}
	if patch.Selfstar != nil {
		set["selfstar"] = *patch.Selfstar
	}
	if patch.IgnoreBots != nil {
		set["ignorebots"] = *patch.IgnoreBots
	}
	if patch.MinimumStars != nil {
		if *patch.MinimumStars < 1 {
			return errors.New("minimum stars must be >= 1")
		}
		set["stars"] = *patch.MinimumStars
	}

	if len(set) == 1 { // only updated_at — nothing to write
		return nil
	}

	return g.cacheAndReturn(ctx, guildID, bson.M{"$set": set})
}

func (g *Guilds) Delete(ctx context.Context, guildID string) error {
	if _, err := g.col.DeleteOne(ctx, bson.M{"guild_id": guildID}); err != nil {
		return fmt.Errorf("store: removing guild %s: %w", guildID, err)
	}
	g.cache.delete(guildID)
	return nil
}

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

// SetChannelStars updates the per-channel star requirement for channelID,
// appending a new entry when one isn't configured yet. Both cases are handled
// in a single atomic update.
func (g *Guilds) SetChannelStars(ctx context.Context, guildID, channelID string, stars int) error {
	res := g.col.FindOneAndUpdate(
		ctx,
		bson.M{"guild_id": guildID},
		bson.A{
			bson.M{"$set": bson.M{
				"updated_at": time.Now(),
				"channel_settings": bson.M{
					"$cond": bson.A{
						bson.M{"$in": bson.A{channelID, "$channel_settings.id"}},
						bson.M{"$map": bson.M{
							"input": "$channel_settings",
							"as":    "setting",
							"in": bson.M{"$cond": bson.A{
								bson.M{"$eq": bson.A{"$$setting.id", channelID}},
								bson.M{"id": channelID, "star_requirement": stars},
								"$$setting",
							}},
						}},
						bson.M{"$concatArrays": bson.A{
							bson.M{"$ifNull": bson.A{"$channel_settings", bson.A{}}},
							bson.A{bson.M{"id": channelID, "star_requirement": stars}},
						}},
					},
				},
			}},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)

	guild := &Guild{}
	if err := res.Decode(guild); err != nil {
		return fmt.Errorf("store: setting channel stars for %s in guild %s: %w", channelID, guildID, err)
	}

	g.cache.set(guild)
	return nil
}

func (g *Guilds) UnsetChannelStars(ctx context.Context, guildID, channelID string) error {
	return g.cacheAndReturn(ctx, guildID, bson.M{"$pull": bson.M{"channel_settings": bson.M{"id": channelID}}})
}

// createIndex creates the guilds unique index.
func (g *Guilds) createIndex(ctx context.Context) error {
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

	g.cache.set(guild)
	return nil
}

func (c *guildCache) set(g *Guild) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.guilds[g.ID] = g
}

func (c *guildCache) delete(guildID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.guilds, guildID)
}

func (c *guildCache) get(guildID string) *Guild {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.guilds[guildID]
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

	fmt.Fprintf(&sb, "<#%v>``%v``: %v ", g.ChannelSettings[0].ID, g.ChannelSettings[0].ID, g.ChannelSettings[0].StarRequirement)

	inRow := 1
	if len(g.ChannelSettings) > 1 {
		for _, ch := range g.ChannelSettings[1:] {
			if inRow == 2 {
				fmt.Fprintf(&sb, "\n<#%v>``%v``: %v ", ch.ID, ch.ID, ch.StarRequirement)
				inRow = 0
			} else {
				fmt.Fprintf(&sb, "| <#%v>``%v``: %v ", ch.ID, ch.ID, ch.StarRequirement)
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
	fmt.Fprintf(&sb, "<#%v>``%v`` ", g.BannedChannels[0], g.BannedChannels[0])

	inRow := 1
	if len(g.BannedChannels) > 1 {
		for _, ch := range g.BannedChannels[1:] {
			if inRow == 2 {
				fmt.Fprintf(&sb, "\n<#%v>``%v`` ", ch, ch)
				inRow = 0
			} else {
				fmt.Fprintf(&sb, "| <#%v>``%v``", ch, ch)
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
