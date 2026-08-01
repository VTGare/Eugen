package embed

import (
	"fmt"

	"github.com/VTGare/Eugen/store"
	"github.com/VTGare/embeds"
	"github.com/bwmarrin/discordgo"
)

// starEmojiURL is the CDN URL for the default star emoji icon used in
// starboard footers when no custom guild emoji is configured.
const starEmojiURL = "https://cdn.jsdelivr.net/gh/twitter/twemoji@14.0.3/assets/72x72/2b50.png"

// EmbedBuilder constructs a starboard embed for a Discord message that has
// reached the reaction threshold. It encapsulates all media extraction,
// content modification, and embed formatting logic so the starboard engine
// can focus on the reaction-count lifecycle.
type EmbedBuilder struct {
	guild *store.Guild
}

// NewEmbedBuilder creates a builder bound to the given guild settings.
func NewEmbedBuilder(guild *store.Guild) *EmbedBuilder {
	return &EmbedBuilder{guild: guild}
}

// BuildResult holds everything needed to send a starboard message.
type BuildResult struct {
	Send *discordgo.MessageSend
}

// Build produces the starboard message send payload for a Discord message
// that has reached the reaction threshold. The effectiveCount is the
// post-adjustment reaction count (after self-star exclusion if applicable).
// selfStar indicates whether the message author reacted themselves.
// Returns nil if the message has no content and no media.
func (b *EmbedBuilder) Build(ch *discordgo.Channel, message *discordgo.Message, react *discordgo.MessageReactions, effectiveCount int, selfStar bool) (*BuildResult, error) {
	var (
		eb         = embeds.NewBuilder()
		messageURL = fmt.Sprintf("https://discord.com/channels/%v/%v/%v", message.GuildID, message.ChannelID, message.ID)
	)

	eb.Author(
		fmt.Sprintf("@%v in #%v", message.Author.Username, ch.Name),
		messageURL, message.Author.AvatarURL(""),
	)
	eb.Color(int(b.guild.EmbedColour))
	eb.Timestamp(message.Timestamp)
	eb.AddField("Original message", fmt.Sprintf("[Click here](%v)", messageURL), true)

	footerText := fmt.Sprintf("%v", effectiveCount)
	if selfStar {
		footerText += " | self-starred"
	}
	eb.Footer(footerText, b.footerIcon(react))

	content, file, hasMedia, err := b.buildContent(eb, message)
	if err != nil {
		return nil, err
	}

	// Return nil if the message has no actual content and no media to show.
	if content == "" && file == nil && !hasMedia {
		return nil, nil
	}

	eb.Description(content)
	embed := eb.Finalize()

	send := &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
	}
	if file != nil {
		send.Files = []*discordgo.File{file}
	}

	return &BuildResult{Send: send}, nil
}

// footerIcon returns the CDN URL for the reaction emoji if it's a guild
// custom emoji, otherwise the default star emoji URL.
func (b *EmbedBuilder) footerIcon(react *discordgo.MessageReactions) string {
	if b.guild.IsGuildEmoji() && react != nil && react.Emoji != nil {
		return emojiURL(react.Emoji)
	}
	return starEmojiURL
}

// UpdateFooter rewrites the footer text and icon on an existing starboard
// embed to reflect the new reaction count. This avoids rebuilding the
// entire embed on every count change.
func (b *EmbedBuilder) UpdateFooter(embed *discordgo.MessageEmbed, count int, selfStar bool, react *discordgo.MessageReactions) {
	text := fmt.Sprintf("%v", count)
	if selfStar {
		text += " | self-starred"
	}
	if embed.Footer == nil {
		embed.Footer = &discordgo.MessageEmbedFooter{}
	}
	embed.Footer.Text = text
	embed.Footer.IconURL = b.footerIcon(react)
}

// buildContent runs the media extraction pipeline and returns the final
// message content string, any file to upload, whether media (image/file)
// was found, and an error.
func (b *EmbedBuilder) buildContent(eb *embeds.Builder, message *discordgo.Message) (string, *discordgo.File, bool, error) {
	mr := extract(eb, message)
	if mr.err != nil {
		return "", nil, false, mr.err
	}

	content := message.Content
	hasMedia := mr.hasImage || mr.file != nil

	// Handle forwarded messages (message snapshots).
	if len(message.MessageSnapshots) > 0 {
		fmsg := message.MessageSnapshots[0].Message
		content = fmsg.Content
		snapshotFile, snapshotModify, snapshotHasImage, err := b.extractSnapshot(eb, fmsg)
		if err != nil {
			return "", nil, false, err
		}
		if snapshotModify != nil {
			content = snapshotModify(content)
		}
		hasMedia = hasMedia || snapshotHasImage || snapshotFile != nil
		eb.AddField("Forwarded message", fmt.Sprintf(
			"[Click here](https://discord.com/channels/%v/%v/%v)",
			message.MessageReference.GuildID,
			message.MessageReference.ChannelID,
			message.MessageReference.MessageID,
		))
		if snapshotFile != nil {
			return content, snapshotFile, hasMedia, nil
		}
	}

	if mr.modify != nil {
		content = mr.modify(content)
	}

	// Append reply reference if the message is a reply.
	if ref := message.ReferencedMessage; ref != nil {
		content += "\n\n> Replying to <@" + ref.Author.ID + ">"
		if ref.Content != "" {
			content += "\n> \n> " + ref.Content
		} else {
			replyURL := fmt.Sprintf(
				"[Click here](https://discord.com/channels/%v/%v/%v)",
				message.GuildID,
				message.MessageReference.ChannelID,
				message.MessageReference.MessageID,
			)
			eb.AddField("Reply to", replyURL)
		}
	}

	return content, mr.file, hasMedia, nil
}

func (b *EmbedBuilder) extractSnapshot(eb *embeds.Builder, fmsg *discordgo.Message) (*discordgo.File, func(string) string, bool, error) {
	mr := extract(eb, fmsg)
	if mr.err != nil {
		return nil, nil, false, mr.err
	}
	return mr.file, mr.modify, mr.hasImage, nil
}

func emojiURL(emoji *discordgo.Emoji) string {
	u := fmt.Sprintf("https://cdn.discordapp.com/emojis/%v.", emoji.ID)
	if emoji.Animated {
		u += "gif"
	} else {
		u += "png"
	}
	return u
}
