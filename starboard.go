package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/VTGare/Eugen/database"
	"github.com/VTGare/Eugen/services"
	"github.com/VTGare/Eugen/utils"
	"github.com/VTGare/embeds"
	"github.com/bwmarrin/discordgo"
	"github.com/sirupsen/logrus"
	"mvdan.cc/xurls/v2"
)

type StarboardEvent struct {
	React       *discordgo.MessageReactions
	guild       *database.Guild
	session     *discordgo.Session
	message     *discordgo.Message
	board       *database.Message
	addEvent    *discordgo.MessageReactionAdd
	removeEvent *discordgo.MessageReactionRemove
	deleteEvent *discordgo.MessageDelete
	selfstar    bool
}

type StarboardFile struct {
	Name      string
	URL       string
	Thumbnail *os.File
	Resp      *http.Response
}

func newStarboardEventAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd, msg *discordgo.Message, emote *discordgo.MessageReactions) (*StarboardEvent, error) {
	guild := database.GuildCache[r.GuildID]
	se := &StarboardEvent{guild: guild, message: msg, session: s, addEvent: r, removeEvent: nil, React: emote}

	return se, nil
}

func newStarboardEventRemove(s *discordgo.Session, r *discordgo.MessageReactionRemove, msg *discordgo.Message) (*StarboardEvent, error) {
	guild := database.GuildCache[r.GuildID]

	emote := FindReact(msg, guild.StarEmote)
	se := &StarboardEvent{guild: guild, message: msg, session: s, addEvent: nil, removeEvent: r, React: emote}

	return se, nil
}

func newStarboardEventDeleted(s *discordgo.Session, d *discordgo.MessageDelete) (*StarboardEvent, error) {
	guild := database.GuildCache[d.GuildID]

	return &StarboardEvent{guild: guild, message: &discordgo.Message{ID: d.ID, ChannelID: d.ChannelID}, session: s, addEvent: nil, removeEvent: nil, deleteEvent: d}, nil
}

func (se *StarboardEvent) Run() error {
	var err error

	se.board, err = database.Repost(se.message.ChannelID, se.message.ID)
	if err != nil {
		return err
	}

	if se.deleteEvent != nil {
		se.deleteStarboard()
	} else if se.isStarboarded() {
		self, err := se.isSelfStar()
		if err != nil {
			return err
		}
		se.selfstar = self

		switch {
		case se.addEvent != nil:
			se.incrementStarboard()
		case se.removeEvent != nil:
			se.decrementStarboard()
		}
	} else if se.addEvent != nil {
		self, err := se.isSelfStar()
		if err != nil {
			return err
		}
		se.selfstar = self

		se.createStarboard()
	}

	return nil
}

func (se *StarboardEvent) isStarboarded() bool {
	return se.board != nil
}

func (se *StarboardEvent) isSelfStar() (bool, error) {
	if se.React == nil {
		return false, nil
	}

	users, err := se.session.MessageReactions(se.message.ChannelID, se.message.ID, se.React.Emoji.APIName(), 100, "", "")
	if err != nil {
		return false, fmt.Errorf("MessageReactions(): %v", err)
	}

	for _, user := range users {
		if user.ID == se.message.Author.ID {
			return true, nil
		}
	}

	return false, nil
}

func (se *StarboardEvent) createStarboard() error {
	var (
		react    = se.React
		required = se.guild.StarsRequired(se.addEvent.ChannelID)
	)

	if react == nil {
		return nil
	}

	if se.selfstar && !se.guild.Selfstar {
		react.Count--
	}

	if react.Count < required {
		return nil
	}

	ch, err := se.session.Channel(se.message.ChannelID)
	if err != nil {
		return err
	}

	embed, err := createEmbed(se.guild, ch, se.message, react)
	if err != nil {
		return err
	}

	if embed == nil {
		return nil
	}

	log := logrus.WithFields(logrus.Fields{
		"guild":   se.guild.ID,
		"channel": se.addEvent.ChannelID,
		"message": se.addEvent.MessageID,
	})

	log.Debug("creating a new starboard")

	starboardChannel := ""
	if ch.NSFW && se.guild.NSFWStarboardChannel != "" {
		starboardChannel = se.guild.NSFWStarboardChannel
	} else {
		starboardChannel = se.guild.StarboardChannel
	}

	starboard, err := se.session.ChannelMessageSendComplex(starboardChannel, embed)
	if err != nil {
		return err
	}

	handleError(se.session, se.addEvent.ChannelID, err)
	oPair := database.NewPair(se.message.ChannelID, se.message.ID)
	sPair := database.NewPair(starboard.ChannelID, starboard.ID)
	err = database.InsertOneMessage(database.NewMessage(&oPair, &sPair, se.addEvent.GuildID))
	handleError(se.session, se.addEvent.ChannelID, err)

	return nil
}

func (se *StarboardEvent) incrementStarboard() {
	if react := se.React; react != nil {
		if se.selfstar && !se.guild.Selfstar {
			react.Count--
		}

		msg, err := se.session.ChannelMessage(se.board.Starboard.ChannelID, se.board.Starboard.MessageID)
		if err != nil {
			if strings.Contains(err.Error(), "404 Not Found") {
				logrus.Infoln("Unknown starboard cached. Removing.")
				err := database.DeleteMessage(&database.MessagePair{ChannelID: se.message.ChannelID, MessageID: se.message.ID})
				if err != nil {
					logrus.Warnln("database.DeleteMessage(): ", err)
				}
				return
			}
			logrus.Warnln("se.session.ChannelMessage(): ", err)
		} else {
			embed := se.editStarboard(msg, react)
			if embed != nil {
				logrus.Infoln(fmt.Sprintf("Editing starboard (adding) %v in channel %v", msg.ID, msg.ChannelID))
				se.session.ChannelMessageEditEmbed(msg.ChannelID, msg.ID, embed)
			}
		}
	}
}

func (se *StarboardEvent) decrementStarboard() {
	starboard, err := se.session.ChannelMessage(se.board.Starboard.ChannelID, se.board.Starboard.MessageID)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			logrus.Infoln("Unknown starboard cached. Removing.")
			err := database.DeleteMessage(&database.MessagePair{ChannelID: se.message.ChannelID, MessageID: se.message.ID})
			if err != nil {
				logrus.Warnln("database.DeleteMessage(): ", err)
			}
			return
		}
		logrus.Warnln("se.session.ChannelMessage(): ", err)
	}

	if starboard == nil {
		logrus.Warnln("decrementStarboard(): nil starboard")
		return
	}

	required := se.guild.StarsRequired(se.removeEvent.ChannelID)
	if react := se.React; react != nil {
		if se.selfstar && !se.guild.Selfstar {
			react.Count--
		}

		if react.Count <= required/2 {
			err := se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
			if err != nil {
				logrus.Warnln("se.session.ChannelMessageDelete():", err)
			}
		} else {
			embed := se.editStarboard(starboard, react)
			if embed != nil {
				logrus.Infof("Editing starboard (subtracting) %v in channel %v", se.board.Starboard.MessageID, se.board.Starboard.ChannelID)
				_, err := se.session.ChannelMessageEditEmbed(starboard.ChannelID, starboard.ID, embed)
				if err != nil {
					logrus.Warnln("se.session.ChannelMessageEditEmbed():", err)
				}
			}
		}
	} else {
		err := se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
		if err != nil {
			logrus.Warnln("se.session.ChannelMessageDelete(): ", err)
		}
	}
}

func (se *StarboardEvent) deleteStarboard() error {
	original := true

	if se.board == nil {
		original = false
		board, err := database.RepostByStarboard(se.deleteEvent.ChannelID, se.message.ID)
		if err != nil {
			return err
		}
		if board != nil {
			se.board = board
		} else {
			return nil
		}
	}

	if ch, ok := starboardQueue[*se.board.Original]; ok {
		close(ch)
		delete(starboardQueue, *se.board.Original)
	}

	err := database.DeleteMessage(se.board.Original)
	if err != nil {
		logrus.Warnln("database.DeleteMessage():", err)
	}

	logrus.Infof("Deleting starboard. ID: %v. Original: %v", se.deleteEvent.ID, original)
	if original {
		starboard, err := se.session.ChannelMessage(se.board.Starboard.ChannelID, se.board.Starboard.MessageID)
		if err != nil {
			return err
		}
		err = se.session.ChannelMessageDelete(starboard.ChannelID, starboard.ID)
		if err != nil {
			logrus.Warnln("se.session.ChannelMessageDelete():", err)
		}
	}
	return nil
}

func createEmbed(
	guild *database.Guild, ch *discordgo.Channel, message *discordgo.Message,
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

		eb.AddField("Forwarded message", fmt.Sprintf("[Click here](https://discord.com/channels/%v/%v/%v)",
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
			url := fmt.Sprintf("https://discord.com/channels/%v/%v/%v",
				message.ReferencedMessage.GuildID,
				message.ReferencedMessage.ChannelID,
				message.ReferencedMessage.ID,
			)
			eb.AddField("Reply to", url)
		}
	}

	eb.Description(content)
	embed := eb.Finalize()
	msg.Embeds = []*discordgo.MessageEmbed{embed}

	return msg, nil
}

type modifyContentFunc func(content string) string

func messageContent(eb *embeds.Builder, message *discordgo.Message) (*discordgo.File, modifyContentFunc, error) {
	// Apply sticker first. Anything else will override it.
	if len(message.StickerItems) != 0 {
		sticker := message.StickerItems[0]
		url := fmt.Sprintf("https://cdn.discordapp.com/stickers/%v.png", sticker.ID)
		eb.Image(url)
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

func fromURL(eb *embeds.Builder, message *discordgo.Message, url *EugenURL) (*discordgo.File, modifyContentFunc, error) {
	removeURL := func(content string) string {
		return strings.Replace(content, url.URL.String(), "", 1)
	}

	if url.Type == URLTypeImage {
		eb.Image(url.URL.String())
		return nil, removeURL, nil
	}

	if url.Type == URLTypeVideo {
		uri := url.URL.String()
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
	}

	if url.Type == URLTypeTenor {
		uri := url.URL.String()
		res, err := services.Tenor(uri)
		if err != nil {
			return nil, nil, fmt.Errorf("tenor error: %w", err)
		}

		// Do nothing.
		if len(res.Media) == 0 {
			return nil, nil, nil
		}

		eb.Image(res.Media[0].MediumGIF.URL)
		return nil, removeURL, nil
	}

	if url.Type == URLTypeImgur {
		eb.Image(fmt.Sprintf("https://i.imgur.com/%v.png", url.URL.Path))
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
		case strings.Contains(parsed.String(), "tenor.com/view"):
			eu.Type = URLTypeTenor
		default:
			continue
		}

		urls = append(urls, eu)
	}

	return urls
}

func FindReact(message *discordgo.Message, emote string) *discordgo.MessageReactions {
	for _, react := range message.Reactions {
		if strings.ToLower(react.Emoji.APIName()) == strings.Trim(emote, "<:>") {
			return react
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

	if se.guild.IsGuildEmoji() {
		embed.Footer.Text = strconv.Itoa(react.Count)
	} else {
		embed.Footer.Text = fmt.Sprintf("⭐ %v", react.Count)
	}

	if se.selfstar && se.guild.Selfstar {
		embed.Footer.Text += " | self-starred"
	}

	return embed
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

// download file downloads a file from URL and returns its contents and filename.
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
	url := fmt.Sprintf("https://cdn.discordapp.com/emojis/%v.", emoji.ID)
	if emoji.Animated {
		url += "gif"
	} else {
		url += "png"
	}

	return url
}

func hasSuffixes(str string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(str, suffix) {
			return true
		}
	}

	return false
}
