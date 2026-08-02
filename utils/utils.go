package utils

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	// EmbedColor is a default Discord embed color
	EmbedColor = 16744576
	// ErrNotEnoughArguments is a default error when not enough arguments were given
	ErrNotEnoughArguments = errors.New("not enough arguments")
	// ErrParsingArgument is a default error when provided arguments couldn't be parsed
	ErrParsingArgument = errors.New("error parsing arguments, please make sure all arguments are integers")
	// ErrNoPermission is a default error when user doesn't have enough permissions to execute a command
	ErrNoPermission = errors.New("you don't have enough permission to execute this command")
)

var (
	// imageExtensions covers all image formats Discord supports for embeds:
	// https://discord.com/developers/docs/reference#image-resource-limits
	imageExtensions = []string{
		".apng", ".avif", ".bmp", ".gif", ".jpg", ".jpeg",
		".png", ".svg", ".tiff", ".tif", ".webp",
	}
	// videoExtensions covers all video formats Discord supports for embeds.
	videoExtensions = []string{
		".mp4", ".webm", ".mov", ".avi", ".mkv", ".wmv", ".mpg", ".mpeg", ".gifv",
	}
)

func IsImageURL(uri string) bool {
	return hasExtension(uri, imageExtensions)
}

func IsVideoURL(uri string) bool {
	return hasExtension(uri, videoExtensions)
}

func hasExtension(uri string, exts []string) bool {
	parsed, err := url.Parse(uri)
	if err != nil {
		return false
	}
	ext := strings.ToLower(path.Ext(parsed.Path))
	return slices.Contains(exts, ext)
}

// EmbedTimestamp returns currect time formatted to RFC3339 for Discord embeds
func EmbedTimestamp() string {
	return time.Now().Format(time.RFC3339)
}

// MemberHasPermission checks if guild member has a permission to do something on a server.
func MemberHasPermission(s *discordgo.Session, guildID string, userID string, permission int64) (bool, error) {
	member, err := s.State.Member(guildID, userID)
	if err != nil {
		if member, err = s.GuildMember(guildID, userID); err != nil {
			return false, err
		}
	}

	g, err := s.Guild(guildID)
	if err != nil {
		return false, err
	}

	if g.OwnerID == userID {
		return true, nil
	}

	// Iterate through the role IDs stored in member.Roles
	// to check permissions
	for _, roleID := range member.Roles {
		role, err := s.State.Role(guildID, roleID)
		if err != nil {
			return false, err
		}

		if role.Permissions&permission != 0 {
			return true, nil
		}
	}

	return false, nil
}

func IsValidChannel(s *discordgo.Session, guildID string, channelID string) bool {
	ch, err := s.Channel(channelID)
	if err != nil {
		slog.Warn("validating channel", "err", err)
		return false
	}

	if ch.GuildID == guildID {
		return true
	}

	return false
}

// FormatBool returns human-readable representation of boolean
func FormatBool(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func FormatChannel(id string) string {
	if id == "" {
		return "-"
	}

	if strings.HasPrefix(id, "<#") {
		return id
	}

	return fmt.Sprintf("<#%v>", id)
}

// GetEmoji returns a guild emoji API name from Discord state
func GetEmoji(s *discordgo.Session, guildID, e string) (string, error) {
	emojis, err := s.GuildEmojis(guildID)
	if err != nil {
		return "", err
	}

	for _, emoji := range emojis {
		if str := fmt.Sprintf("<:%v>", strings.ToLower(emoji.APIName())); str == e {
			return str, nil
		}
	}

	return e, nil
}

func Map(vs []string, f func(string) string) []string {
	vsm := make([]string, len(vs))
	for i, v := range vs {
		vsm[i] = f(v)
	}
	return vsm
}

func BaseEmbed(s *discordgo.Session) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Thumbnail: &discordgo.MessageEmbedThumbnail{URL: s.State.User.AvatarURL("")},
		Color:     EmbedColor,
		Timestamp: EmbedTimestamp(),
	}
}

func CreatePrompt(s *discordgo.Session, m *discordgo.MessageCreate, embed *discordgo.MessageEmbed) string {
	prompt, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	if err != nil {
		slog.Warn("creating prompt", "err", err)
		return ""
	}

	var msg *discordgo.MessageCreate
	for {
		select {
		case m := <-nextMessageCreate(s):
			msg = m
		case <-time.After(2 * time.Minute):
			s.ChannelMessageDelete(prompt.ChannelID, prompt.ID)
			return ""
		}

		if msg.Author.ID != m.Author.ID {
			continue
		}

		s.ChannelMessageDelete(prompt.ChannelID, prompt.ID)
		return msg.Content
	}
}

func nextMessageCreate(s *discordgo.Session) chan *discordgo.MessageCreate {
	out := make(chan *discordgo.MessageCreate)
	s.AddHandlerOnce(func(_ *discordgo.Session, e *discordgo.MessageCreate) {
		out <- e
	})

	return out
}
