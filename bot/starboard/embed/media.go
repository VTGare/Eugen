package embed

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/VTGare/Eugen/utils"
	"github.com/VTGare/embeds"
	"github.com/bwmarrin/discordgo"
	"mvdan.cc/xurls/v2"
)

// maxFileSize is the Discord upload limit for bot-sent files (8 MB).
const maxFileSize int64 = 8_388_608

// mediaResult holds the file (if any) to upload, a function to
// post-process the message content, and whether an image was set on
// the embed builder (for stickers/URLs/embed images that aren't file uploads).
type mediaResult struct {
	file     *discordgo.File
	modify   func(content string) string
	hasImage bool
	err      error
}

// extract determines the dominant media for a message and returns either
// a downloadable file, a content modifier, or both.
func extract(eb *embeds.Builder, message *discordgo.Message) mediaResult {
	// Stickers take lowest priority — anything else overrides.
	if len(message.StickerItems) != 0 {
		sticker := message.StickerItems[0]
		eb.Image(stickerURL(sticker))
		return mediaResult{hasImage: true}
	}

	// Attachments always win over URL-embed extraction.
	if len(message.Attachments) > 0 {
		return fromAttachments(eb, message)
	}

	// Single image/video URL on its own.
	if urls := findURLs(message.Content); len(urls) > 0 {
		return fromURL(eb, message, urls[0])
	}

	// Rich embed (YouTube link, etc).
	if len(message.Embeds) > 0 {
		return fromEmbed(eb, message.Embeds[0])
	}

	return mediaResult{}
}

func fromAttachments(eb *embeds.Builder, message *discordgo.Message) mediaResult {
	var (
		first  = message.Attachments[0]
		rest   = message.Attachments[1:]
		result mediaResult
	)

	if utils.IsImageURL(first.URL) {
		eb.Image(first.URL)
		result.modify = func(content string) string {
			return strings.Replace(content, first.URL, "", 1)
		}
		result.hasImage = true
	} else {
		file, err := downloadFile(first.URL)
		if err != nil {
			return mediaResult{err: err}
		}
		if file == nil {
			eb.AddField("Attachment", fmt.Sprintf("[Click here](%v)", first.URL), true)
		}
		result.file = file
	}

	for i, a := range rest {
		eb.AddField(fmt.Sprintf("Attachment %d", i+2), fmt.Sprintf("[Click here](%v)", a.URL), true)
	}

	return result
}

func fromURL(eb *embeds.Builder, message *discordgo.Message, eugURL *EugenURL) mediaResult {
	uri := eugURL.URL.String()

	removeURL := func(content string) string {
		return strings.Replace(content, uri, "", 1)
	}

	switch eugURL.Type {
	case URLTypeImage:
		eb.Image(uri)
		return mediaResult{modify: removeURL, hasImage: true}
	case URLTypeVideo:
		videoURL := uri
		if strings.HasSuffix(videoURL, "gifv") {
			videoURL = strings.Replace(videoURL, "gifv", "mp4", 1)
		}
		file, err := downloadFile(videoURL)
		if err != nil {
			return mediaResult{err: err}
		}
		if file == nil {
			eb.AddField("Attachment", fmt.Sprintf("[Click here](%v)", uri), true)
		}
		return mediaResult{file: file, modify: removeURL}
	case URLTypeImgur:
		eb.Image(fmt.Sprintf("https://i.imgur.com/%v.png", eugURL.URL.Path))
		if len(message.Embeds) == 0 {
			return mediaResult{modify: removeURL, hasImage: true}
		}
		if message.Embeds[0].Thumbnail != nil {
			eb.Image(message.Embeds[0].Thumbnail.ProxyURL)
		}
		return mediaResult{modify: removeURL, hasImage: true}
	default:
		return mediaResult{}
	}
}

func fromEmbed(eb *embeds.Builder, embed *discordgo.MessageEmbed) mediaResult {
	mr := mediaResult{}
	if embed.Image != nil {
		eb.Image(embed.Image.URL)
		mr.hasImage = true
	}
	if embed.Thumbnail != nil {
		eb.Image(embed.Thumbnail.ProxyURL)
		mr.hasImage = true
	}

	var file *discordgo.File
	if embed.Video != nil {
		eb.AddField("Embedded video", fmt.Sprintf("[Click here](%v)", embed.Video.URL), true)
	}

	mr.file = file
	mr.modify = func(content string) string {
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
	return mr
}

func findURLs(content string) []*EugenURL {
	var (
		rx  = xurls.Strict()
		out []*EugenURL
	)

	for _, uri := range rx.FindAllString(content, -1) {
		parsed, err := url.Parse(uri)
		if err != nil {
			continue
		}

		eu := &EugenURL{URL: parsed}

		switch {
		case utils.IsImageURL(uri):
			eu.Type = URLTypeImage
		case utils.IsVideoURL(uri):
			eu.Type = URLTypeVideo
		case strings.Contains(parsed.Host, "imgur"):
			eu.Type = URLTypeImgur
		default:
			continue
		}

		out = append(out, eu)
	}

	return out
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

	return &discordgo.File{Name: filename, Reader: content}, nil
}

func checkFilesizeLimit(uri string) (bool, error) {
	var limit int64 = maxFileSize

	resp, err := http.Head(uri)
	if err != nil {
		return false, fmt.Errorf("http head: %w", err)
	}

	return resp.ContentLength < limit, nil
}

// getFile downloads a file from URL and returns its contents and filename.
func getFile(uri string) (*bytes.Buffer, string, error) {
	var filename string

	lastSlash := strings.LastIndex(uri, "/")
	querySep := strings.LastIndex(uri, "?")

	if querySep != -1 && querySep > lastSlash {
		filename = uri[lastSlash:querySep]
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
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return nil, "", fmt.Errorf("io copy: %w", err)
	}

	return &buf, filename, nil
}

// stickerURL returns the CDN URL for a Discord sticker based on its
// format type.
func stickerURL(sticker *discordgo.StickerItem) string {
	ext := ".png"
	if sticker.FormatType == discordgo.StickerFormatTypeGIF {
		ext = ".gif"
	}

	return fmt.Sprintf("https://cdn.discordapp.com/stickers/%v%v", sticker.ID, ext)
}
