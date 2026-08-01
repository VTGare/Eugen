package bot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/VTGare/Eugen/store"
	"github.com/VTGare/Eugen/utils"
	"github.com/VTGare/embeds"
	"github.com/bwmarrin/discordgo"
	"mvdan.cc/xurls/v2"
)

// StarboardEvent represents a starboard operation to be processed.
type StarboardEvent struct {
	React       *discordgo.MessageReactions
	Guild       *store.Guild
	Session     *discordgo.Session
	Message     *discordgo.Message
	Board       *store.Message
	AddEvent    *discordgo.MessageReactionAdd
	RemoveEvent *discordgo.MessageReactionRemove
	DeleteEvent *discordgo.MessageDelete
	Selfstar    bool
	Store       *store.Store
	Bot         *Bot
	log         *slog.Logger
}

// Run executes the starboard event logic.
func (se *StarboardEvent) Run() error {
	var err error

	se.Board, err = se.Store.Messages.Repost(context.Background(), se.Message.ChannelID, se.Message.ID)
	if err != nil {
		return err
	}

	if se.DeleteEvent != nil {
		return se.deleteStarboard()
	} else if se.isStarboarded() {
		self, err := se.isSelfStar()
		if err != nil {
			return err
		}
		se.Selfstar = self

		switch {
		case se.AddEvent != nil:
			se.incrementStarboard()
		case se.RemoveEvent != nil:
			se.decrementStarboard()
		}
	} else if se.AddEvent != nil {
		self, err := se.isSelfStar()
		if err != nil {
			return err
		}
		se.Selfstar = self

		return se.createStarboard()
	}

	return nil
}

func (se *StarboardEvent) isStarboarded() bool {
	return se.Board != nil
}

func (se *StarboardEvent) isSelfStar() (bool, error) {
	if se.React == nil {
		return false, nil
	}

	users, err := se.Session.MessageReactions(se.Message.ChannelID, se.Message.ID, se.React.Emoji.APIName(), 100, "", "")
	if err != nil {
		return false, fmt.Errorf("MessageReactions(): %v", err)
	}

	for _, user := range users {
		if user.ID == se.Message.Author.ID {
			return true, nil
		}
	}

	return false, nil
}

func (se *StarboardEvent) createStarboard() error {
	var (
		react    = se.React
		required = se.Guild.StarsRequired(se.AddEvent.ChannelID)
	)

	if react == nil {
		return nil
	}

	if se.Selfstar && !se.Guild.Selfstar {
		react.Count--
	}

	if react.Count < required {
		return nil
	}

	ch, err := se.Session.Channel(se.Message.ChannelID)
	if err != nil {
		return err
	}

	embed, err := createEmbed(se.Guild, ch, se.Message, react)
	if err != nil {
		return err
	}

	if embed == nil {
		return nil
	}

	l := se.log.With(
		"channel_id", se.AddEvent.ChannelID,
		"message_id", se.AddEvent.MessageID,
	)

	l.Debug("creating new starboard")
	starboardChannel := ""
	if ch.NSFW && se.Guild.NSFWStarboardChannel != "" {
		starboardChannel = se.Guild.NSFWStarboardChannel
	} else {
		starboardChannel = se.Guild.StarboardChannel
	}

	starboard, err := se.Session.ChannelMessageSendComplex(starboardChannel, embed)
	if err != nil {
		return err
	}

	var err2 error
	oPair := store.NewPair(se.Message.ChannelID, se.Message.ID)
	sPair := store.NewPair(starboard.ChannelID, starboard.ID)
	err2 = se.Store.Messages.Insert(context.Background(), store.NewMessage(&oPair, &sPair, se.AddEvent.GuildID))
	se.Bot.HandleError(se.Session, se.AddEvent.ChannelID, err2)

	return nil
}

func (se *StarboardEvent) incrementStarboard() {
	l := se.log

	if react := se.React; react != nil {
		if se.Selfstar && !se.Guild.Selfstar {
			react.Count--
		}

		msg, err := se.Session.ChannelMessage(se.Board.Starboard.ChannelID, se.Board.Starboard.MessageID)
		if err != nil {
			if strings.Contains(err.Error(), "404 Not Found") {
				l.Info("unknown starboard cached, removing")
				err := se.Store.Messages.Delete(context.Background(), &store.MessagePair{ChannelID: se.Message.ChannelID, MessageID: se.Message.ID})
				if err != nil {
					l.Warn("deleting message", "err", err)
				}
				return
			}
			l.Warn("fetching starboard message", "err", err)
		} else {
			embed := se.editStarboard(msg, react)
			if embed != nil {
				l.Info("editing starboard", "starboard_id", msg.ID, "channel_id", msg.ChannelID, "op", "add")
				se.Session.ChannelMessageEditEmbed(msg.ChannelID, msg.ID, embed)
			}
		}
	}
}

func (se *StarboardEvent) decrementStarboard() {
	l := se.log

	starboard, err := se.Session.ChannelMessage(se.Board.Starboard.ChannelID, se.Board.Starboard.MessageID)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			l.Info("unknown starboard cached, removing")
			err := se.Store.Messages.Delete(context.Background(), &store.MessagePair{ChannelID: se.Message.ChannelID, MessageID: se.Message.ID})
			if err != nil {
				l.Warn("deleting message", "err", err)
			}
			return
		}
		l.Warn("fetching starboard message", "err", err)
	}

	if starboard == nil {
		l.Warn("starboard is nil")
		return
	}

	required := se.Guild.StarsRequired(se.RemoveEvent.ChannelID)
	if react := se.React; react != nil {
		if se.Selfstar && !se.Guild.Selfstar {
			react.Count--
		}

		if react.Count <= required/2 {
			err := se.Session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
			if err != nil {
				l.Warn("deleting starboard message", "err", err)
			}
		} else {
			embed := se.editStarboard(starboard, react)
			if embed != nil {
				l.Info("editing starboard", "starboard_id", se.Board.Starboard.MessageID, "channel_id", se.Board.Starboard.ChannelID, "op", "subtract")
				_, err := se.Session.ChannelMessageEditEmbed(starboard.ChannelID, starboard.ID, embed)
				if err != nil {
					l.Warn("editing starboard message", "err", err)
				}
			}
		}
	} else {
		err := se.Session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
		if err != nil {
			l.Warn("deleting starboard message", "err", err)
		}
	}
}

func (se *StarboardEvent) deleteStarboard() error {
	l := se.log
	original := true

	if se.Board == nil {
		original = false
		board, err := se.Store.Messages.RepostByStarboard(context.Background(), se.DeleteEvent.ChannelID, se.Message.ID)
		if err != nil {
			return err
		}
		if board != nil {
			se.Board = board
		} else {
			return nil
		}
	}

	if ch, ok := se.Bot.Queue[*se.Board.Original]; ok {
		close(ch)
		delete(se.Bot.Queue, *se.Board.Original)
	}

	err := se.Store.Messages.Delete(context.Background(), se.Board.Original)
	if err != nil {
		l.Warn("deleting message", "err", err)
	}

	l.Info("deleting starboard", "message_id", se.DeleteEvent.ID, "original", original)
	if original {
		starboard, err := se.Session.ChannelMessage(se.Board.Starboard.ChannelID, se.Board.Starboard.MessageID)
		if err != nil {
			return err
		}
		err = se.Session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
		if err != nil {
			l.Warn("deleting starboard message", "err", err)
		}
	}
	return nil
}

func (se *StarboardEvent) editStarboard(msg *discordgo.Message, react *discordgo.MessageReactions) *discordgo.MessageEmbed {
	embed := msg.Embeds[0]

	current, _ := strconv.Atoi(strings.Trim(embed.Footer.Text, "⭐ "))
	if current == react.Count {
		return nil
	}

	if se.Guild.IsGuildEmoji() {
		embed.Footer.Text = strconv.Itoa(react.Count)
	} else {
		embed.Footer.Text = fmt.Sprintf("⭐ %v", react.Count)
	}

	if se.Selfstar && se.Guild.Selfstar {
		embed.Footer.Text += " | self-starred"
	}

	return embed
}

// --- Media / embed processing ---

type modifyContentFunc func(content string) string

func createEmbed(
	guild *store.Guild, ch *discordgo.Channel, message *discordgo.Message,
	react *discordgo.MessageReactions,
) (*discordgo.MessageSend, error) {
	var (
		eb         = embeds.NewBuilder()
		messageURL = fmt.Sprintf("https://discord.com/channels/%v/%v/%v", message.GuildID, message.ChannelID, message.ID)
		msg        = &discordgo.MessageSend{}
	)

	eb.Author(
		fmt.Sprintf("@%v in #%v", message.Author.Username, ch.Name),
		messageURL, message.Author.AvatarURL(""),
	)
	eb.Color(int(guild.EmbedColour))
	eb.Timestamp(message.Timestamp)
	eb.AddField("Original message", fmt.Sprintf("[Click here](%v)", messageURL), true)

	if guild.IsGuildEmoji() {
		text := fmt.Sprintf("%v", react.Count)
		eb.Footer(text, emojiURL(react.Emoji))
	} else {
		text := fmt.Sprintf("%v %v", "⭐", react.Count)
		eb.Footer(text, "")
	}

	var (
		file          *discordgo.File
		modifyContent modifyContentFunc
		content       string
		err           error
	)

	if len(message.MessageSnapshots) != 0 {
		fmsg := message.MessageSnapshots[0].Message

		content = fmsg.Content
		file, modifyContent, err = messageContent(eb, fmsg)
		eb.AddField("Forwarded message", fmt.Sprintf(
			"[Click here](https://discord.com/channels/%v/%v/%v)",
			message.MessageReference.GuildID,
			message.MessageReference.ChannelID,
			message.MessageReference.MessageID,
		))
	} else {
		content = message.Content
		file, modifyContent, err = messageContent(eb, message)
	}

	if err != nil {
		return nil, err
	}

	if file != nil {
		msg.Files = []*discordgo.File{file}
	}

	if modifyContent != nil {
		content = modifyContent(content)
	}

	if message.ReferencedMessage != nil {
		content += "\n\n> Replying to <@" + message.ReferencedMessage.Author.ID + ">"
		if message.ReferencedMessage.Content != "" {
			content += "\n> \n> " + message.ReferencedMessage.Content
		} else {
			u := fmt.Sprintf(
				"https://discord.com/channels/%v/%v/%v",
				message.ReferencedMessage.GuildID,
				message.ReferencedMessage.ChannelID,
				message.ReferencedMessage.ID,
			)
			eb.AddField("Reply to", u)
		}
	}

	eb.Description(content)
	embed := eb.Finalize()
	msg.Embeds = []*discordgo.MessageEmbed{embed}

	return msg, nil
}

func messageContent(eb *embeds.Builder, message *discordgo.Message) (*discordgo.File, modifyContentFunc, error) {
	// Apply sticker first. Anything else will override it.
	if len(message.StickerItems) != 0 {
		sticker := message.StickerItems[0]
		u := fmt.Sprintf("https://cdn.discordapp.com/stickers/%v.png", sticker.ID)
		eb.Image(u)
	}

	// Prioritize attachments over anything else.
	if len(message.Attachments) != 0 {
		return fromAttachments(eb, message)
	}
	urls := findURLs(message.Content)
	if len(urls) != 0 {
		return fromURL(eb, message, urls[0])
	}

	if len(message.Embeds) != 0 {
		return fromEmbed(eb, message.Embeds[0])
	}

	return nil, nil, nil
}

func fromAttachments(eb *embeds.Builder, message *discordgo.Message) (*discordgo.File, modifyContentFunc, error) {
	var (
		first = message.Attachments[0]
		rest  = message.Attachments[1:]
		file  *discordgo.File
	)

	if utils.ImageURLRegex.MatchString(first.URL) {
		eb.Image(first.URL)
	} else {
		var err error
		file, err = downloadFile(first.URL)
		if err != nil {
			return nil, nil, err
		}
		if file == nil {
			eb.AddField("Attachment", fmt.Sprintf("[Click here](%v)", first.URL), true)
		}
	}

	for ind, a := range rest {
		eb.AddField(fmt.Sprintf("Attachment %v", ind+2), fmt.Sprintf("[Click here](%v)", a.URL), true)
	}

	return file, nil, nil
}

func fromURL(eb *embeds.Builder, message *discordgo.Message, eugURL *EugenURL) (*discordgo.File, modifyContentFunc, error) {
	uri := eugURL.URL.String()

	removeURL := func(content string) string {
		return strings.Replace(content, uri, "", 1)
	}

	switch eugURL.Type {
	case URLTypeImage:
		eb.Image(uri)
		return nil, removeURL, nil
	case URLTypeVideo:
		if strings.HasSuffix(uri, "gifv") {
			uri = strings.Replace(uri, "gifv", "mp4", 1)
		}
		file, err := downloadFile(uri)
		if err != nil {
			return nil, nil, err
		}
		if file == nil {
			eb.AddField("Attachment", fmt.Sprintf("[Click here](%v)", uri), true)
		}
		return file, removeURL, nil
	case URLTypeImgur:
		eb.Image(fmt.Sprintf("https://i.imgur.com/%v.png", uri))
		if len(message.Embeds) == 0 {
			return nil, removeURL, nil
		}
		embed := message.Embeds[0]
		if embed.Thumbnail != nil {
			eb.Image(embed.Thumbnail.ProxyURL)
		}
	}

	// If not one of supported URL types, do nothing.
	return nil, nil, nil
}

func fromEmbed(eb *embeds.Builder, embed *discordgo.MessageEmbed) (*discordgo.File, modifyContentFunc, error) {
	if embed.Image != nil {
		eb.Image(embed.Image.URL)
	}

	if embed.Thumbnail != nil {
		eb.Image(embed.Thumbnail.ProxyURL)
	}

	var file *discordgo.File
	if embed.Video != nil {
		eb.AddField("Embedded video", fmt.Sprintf("[Click here](%v)", embed.Video.URL), true)
	}

	contentFunc := func(content string) string {
		if embed.Description == "" {
			return content
		}

		content += "\n\n"

		if embed.Title != "" {
			content += fmt.Sprintf("> %v", embed.Title)
		} else if embed.Author != nil {
			content += fmt.Sprintf("> %v", embed.Author.Name)
		}

		description := strings.ReplaceAll(embed.Description, "\n", "\n> ")
		content += "\n> \n> " + description

		return content
	}

	return file, contentFunc, nil
}

func findURLs(content string) []*EugenURL {
	var (
		rx   = xurls.Strict()
		urls = make([]*EugenURL, 0)
	)

	for _, uri := range rx.FindAllString(content, -1) {
		parsed, err := url.Parse(uri)
		if err != nil {
			continue
		}

		eu := &EugenURL{
			URL: parsed,
		}

		switch {
		case hasSuffixes(parsed.Path, "jpg", "png", "jpeg", "webp", "gif"):
			eu.Type = URLTypeImage
		case hasSuffixes(parsed.Path, "mp4", "webm", "mov", "gifv"):
			eu.Type = URLTypeVideo
		case strings.Contains(parsed.Host, "imgur"):
			eu.Type = URLTypeImgur
		default:
			continue
		}

		urls = append(urls, eu)
	}

	return urls
}

func downloadFile(uri string) (*discordgo.File, error) {
	allowed, err := checkFilesizeLimit(uri)
	if err != nil {
		return nil, fmt.Errorf("filesize limit: %w", err)
	}

	if !allowed {
		return nil, nil
	}

	content, filename, err := getFile(uri)
	if err != nil {
		return nil, err
	}

	return &discordgo.File{
		Name:   filename,
		Reader: content,
	}, nil
}

func checkFilesizeLimit(uri string) (bool, error) {
	var limit int64 = 8388608

	head, err := http.Head(uri)
	if err != nil {
		return false, fmt.Errorf("http head: %w", err)
	}

	return head.ContentLength < limit, nil
}

// getFile downloads a file from URL and returns its contents and filename.
func getFile(uri string) (*bytes.Buffer, string, error) {
	var filename string

	lastSlash := strings.LastIndex(uri, "/")
	querySeparator := strings.LastIndex(uri, "?")

	if querySeparator != -1 && querySeparator > lastSlash {
		filename = uri[lastSlash:querySeparator]
	} else {
		filename = uri[lastSlash:]
	}

	filename = strings.TrimPrefix(filename, "/")

	resp, err := http.Get(uri)
	if err != nil {
		return nil, "", fmt.Errorf("http get: %w", err)
	}

	defer resp.Body.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("io copy: %w", err)
	}

	return &buf, filename, nil
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

func hasSuffixes(str string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(str, suffix) {
			return true
		}
	}
	return false
}
