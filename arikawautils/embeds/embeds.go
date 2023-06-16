package embeds

import (
	"time"

	"github.com/diamondburned/arikawa/v3/discord"
)

const (
	ColorEugen  discord.Color = 0xff8080
	ColorGreen  discord.Color = 0x5cb85c
	ColorYellow discord.Color = 0xffe620
	ColorBlue   discord.Color = 0x439ef1
	ColorRed    discord.Color = 0xde180c
)

type Builder struct {
	embed *discord.Embed
}

func NewBuilder() *Builder {
	return &Builder{
		embed: &discord.Embed{
			Type:  discord.NormalEmbed,
			Color: ColorEugen,
		},
	}
}

func (eb *Builder) Build() discord.Embed {
	return *eb.embed
}

func (eb *Builder) Title(title string) *Builder {
	eb.embed.Title = title
	return eb
}

func (eb *Builder) Description(desc string) *Builder {
	eb.embed.Description = desc
	return eb
}

func (eb *Builder) URL(url string) *Builder {
	eb.embed.URL = url
	return eb
}

func (eb *Builder) AddField(name, value string, inline ...bool) *Builder {
	i := false
	if len(inline) > 0 {
		i = inline[0]
	}

	eb.embed.Fields = append(eb.embed.Fields, discord.EmbedField{
		Name: name, Value: value, Inline: i,
	})

	return eb
}

func (eb *Builder) Thumbnail(url string) *Builder {
	eb.embed.Thumbnail = &discord.EmbedThumbnail{
		URL: url,
	}

	return eb
}

func (eb *Builder) Image(title string) *Builder {
	return eb
}

func (eb *Builder) Author(name, icon, url string) *Builder {
	eb.embed.Author = &discord.EmbedAuthor{
		Name: name,
		Icon: icon,
		URL:  url,
	}
	return eb
}

func (eb *Builder) Color(color int) *Builder {
	eb.embed.Color = discord.Color(color)
	return eb
}

func (eb *Builder) Timestamp(t time.Time) *Builder {
	eb.embed.Timestamp = discord.NewTimestamp(t)
	return eb
}

func (eb *Builder) Footer(text, icon string) *Builder {
	eb.embed.Footer = &discord.EmbedFooter{
		Text: text,
		Icon: icon,
	}

	return eb
}

func (eb *Builder) ErrorTemplate(message string) *Builder {
	eb.Title("🛑 A wild error appears!").Description(message).Footer("Please use bt!feedback command if something went horribly wrong.", "")
	eb.Color(14555148)
	return eb
}
