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

// maxFileSize is the Discord upload limit for bot-sent files.
const maxFileSize int64 = 10 << 20 // 10 MB

type mediaResult struct {
	file     *discordgo.File
	modify   func(content string) string
	hasImage bool
	err      error
}

// extract processes multiple media types from a message.
// When multiple media types are present, higher-priority media takes
// precedence in conflicts (e.g., setting the embed image).
func extract(eb *embeds.Builder, message *discordgo.Message) mediaResult {
	var result mediaResult

	// Process stickers first. Gets overwritten by anything that comes after.
	if len(message.StickerItems) != 0 {
		sticker := message.StickerItems[0]
		eb.Image(stickerURL(sticker))
		result.hasImage = true
	}

	// Process attachments.
	if len(message.Attachments) > 0 {
		attachmentResult := fromAttachments(eb, message)
		if attachmentResult.err != nil {
			return mediaResult{err: attachmentResult.err}
		}

		result.file = attachmentResult.file
		result.hasImage = attachmentResult.hasImage || result.hasImage
	}

	// Process standalone URLs.
	if urls := findURLs(message.Content); len(urls) > 0 {
		urlResult := fromURL(eb, urls[0])
		if urlResult.err != nil {
			return mediaResult{err: urlResult.err}
		}

		result.file = urlResult.file
		result.hasImage = urlResult.hasImage || result.hasImage
		result.modify = chainModify(result.modify, urlResult.modify)
	}

	// Process embeds.
	if len(message.Embeds) > 0 {
		embedResult := fromEmbed(eb, message.Embeds[0])
		if embedResult.err != nil {
			return mediaResult{err: embedResult.err}
		}

		if result.file == nil {
			result.file = embedResult.file
		}

		result.hasImage = embedResult.hasImage || result.hasImage
		result.modify = chainModify(result.modify, embedResult.modify)
	}

	return result
}

// chainModify chains two modify functions together
func chainModify(first, second func(content string) string) func(content string) string {
	if first == nil && second == nil {
		return nil
	}

	if first == nil {
		return second
	}

	if second == nil {
		return first
	}

	return func(content string) string {
		return second(first(content))
	}
}

func fromAttachments(eb *embeds.Builder, message *discordgo.Message) mediaResult {
	var (
		first  = message.Attachments[0]
		rest   = message.Attachments[1:]
		result mediaResult
	)

	if utils.IsImageURL(first.URL) {
		eb.Image(first.URL)
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

func fromURL(eb *embeds.Builder, eugURL *EugenURL) mediaResult {
	uri := eugURL.URL.String()

	removeURL := func(content string) string {
		return strings.Replace(content, uri, "", 1)
	}

	conditionalRemoveURL := func(eb *embeds.Builder, imageURL string) func(content string) string {
		return func(content string) string {
			embed := eb.Finalize()
			if embed.Image != nil && embed.Image.URL == imageURL {
				return removeURL(content)
			}

			return content
		}
	}

	switch eugURL.Type {
	case URLTypeImage:
		eb.Image(uri)
		return mediaResult{modify: conditionalRemoveURL(eb, uri), hasImage: true}
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
		if embed.Thumbnail.URL != "" {
			eb.Image(embed.Thumbnail.URL)
		} else {
			eb.Image(embed.Thumbnail.ProxyURL)
		}

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
