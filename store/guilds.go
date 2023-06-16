package store

import (
	"context"
	"time"
)

type GuildStore interface {
	Guild(ctx context.Context, guildID string) (*Guild, error)
	CreateGuild(ctx context.Context, guildID string) (*Guild, error)
	UpdateGuild(ctx context.Context, guild *Guild) (*Guild, error)
	DeleteGuild(ctx context.Context, guildID string) error
}

type Guild struct {
	Selfstar   bool      `json:"selfstar" bson:"selfstar"`
	IgnoreBots bool      `json:"ignore_bots" bson:"ignore_bots"`
	EmbedColor int64     `json:"color" bson:"color"`
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
	ID         string    `json:"guild_id" bson:"guild_id"`
	Emote      string    `json:"emote" bson:"emote"`

	Starboards      []*Starboard       `json:"starboards" bson:"starboards"`
	IgnoredUsers    []string           `json:"ignored_users" bson:"ignored_users"`
	ChannelSettings []*ChannelSettings `json:"channel_settings" bson:"channel_settings"`

	Enabled          bool     `json:"enabled" bson:"enabled"`                     // DEPRECATED: doesn't do anything
	OldIgnoreBots    bool     `json:"ignorebots" bson:"ignorebots"`               // DEPRECATED: renamed bson field name
	MinimumStars     int      `json:"stars" bson:"stars"`                         // DEPRECATED: moved to Starboard array
	StarboardChannel string   `json:"starboard" bson:"starboard"`                 // DEPRECATED: moved to Starboard array
	Prefix           string   `json:"prefix" bson:"prefix"`                       // DEPRECATED: using slash commands
	BannedChannels   []string `json:"banned" bson:"banned"`                       // DEPRECATED: moved to channel settings
	BlacklistedUsers []string `json:"blacklisted_users" bson:"blacklisted_users"` // DEPRECATED: renamed to IgnoredUsers

}

type Starboard struct {
	StarboardChannel string   `json:"starboard_channel" bson:"starboard_channel"`
	Channels         []string `json:"channels" bson:"channels"`

	RequiredStars int `json:"required_stars" bson:"required_stars"`
}

type ChannelSettings struct {
	ID            string `json:"id" bson:"id"`
	Ignored       bool   `json:"ignored"`
	RequiredStars int    `json:"required_stars" bson:"required_stars"`

	StarRequirement int `json:"star_requirement" bson:"star_requirement"` // DEPRECATED: renamed
}

func DefaultGuild(guildID string) *Guild {
	return &Guild{
		ID:         guildID,
		Selfstar:   false,
		IgnoreBots: false,
		EmbedColor: 0,
		Emote:      "⭐",

		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}
