package embed_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/Eugen/bot/starboard/embed"
	"github.com/VTGare/Eugen/store"
	"github.com/bwmarrin/discordgo"
)

const (
	guildID   = "100"
	channelID = "200"
	messageID = "300"
	authorID  = "user1"
)

var _ = Describe("EmbedBuilder", func() {
	var (
		builder *embed.EmbedBuilder
		guild   *store.Guild
	)

	BeforeEach(func() {
		guild = store.NewGuild("Test Guild", guildID)
		guild.Enabled = true
		guild.StarboardChannel = "starboard"
		guild.StarEmote = "\u2b50" // ⭐ standard star emoji
		builder = embed.NewEmbedBuilder(guild)
	})

	Describe("Build", func() {
		It("produces a valid embed for text-only messages", func() {
			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
				Content:   "Hello world",
				Timestamp: time.Now(),
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			}, 5, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Send).NotTo(BeNil())
			Expect(result.Send.Embeds).To(HaveLen(1))

			emb := result.Send.Embeds[0]
			Expect(emb.Description).To(ContainSubstring("Hello world"))
			Expect(emb.Footer).NotTo(BeNil())
			Expect(emb.Footer.Text).To(ContainSubstring("5"))
			Expect(emb.Footer.IconURL).To(ContainSubstring("2b50"))
		})

		It("returns nil for empty messages with no content or media", func() {
			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			}, 5, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("includes self-star indicator in footer", func() {
			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Content:   "Hello",
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			}, 5, true)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Send.Embeds[0].Footer.Text).To(ContainSubstring("self-starred"))
		})

		It("appends reply reference to content", func() {
			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Content:   "Hello",
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
				ReferencedMessage: &discordgo.Message{
					Content: "replying to this",
					Author:  &discordgo.User{ID: "author2"},
				},
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			}, 1, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Send.Embeds[0].Description).To(ContainSubstring("Replying to <@author2>"))
			Expect(result.Send.Embeds[0].Description).To(ContainSubstring("replying to this"))
		})

		It("sets guild emoji icon when IsGuildEmoji is true", func() {
			guild.StarEmote = "<:my emoji:999>"
			guild.StarboardChannel = "starboard"

			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Content:   "Hello",
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "999", Name: "myemoji", Animated: false},
			}, 1, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Send.Embeds[0].Footer.IconURL).To(ContainSubstring("cdn.discordapp.com/emojis/999.png"))
		})

		It("sets animated guild emoji icon", func() {
			guild.StarEmote = "<:my emoji:999>"

			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Content:   "Hello",
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "999", Name: "myemoji", Animated: true},
			}, 1, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Send.Embeds[0].Footer.IconURL).To(ContainSubstring("cdn.discordapp.com/emojis/999.gif"))
		})

		It("processes image URLs by setting embed image", func() {
			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
				Content: "Check this image: https://example.com/photo.png",
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			}, 1, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Send.Embeds[0].Image).NotTo(BeNil())
			Expect(result.Send.Embeds[0].Image.URL).To(Equal("https://example.com/photo.png"))
			Expect(result.Send.Embeds[0].Description).NotTo(ContainSubstring("photo.png"))
		})

		It("processes embeds with thumbnail images", func() {
			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
				Content: "Cool video",
				Embeds: []*discordgo.MessageEmbed{
					{
						Thumbnail: &discordgo.MessageEmbedThumbnail{
							ProxyURL: "https://example.com/thumb.jpg",
						},
						Description: "Video description here",
						Title:       "Video Title",
					},
				},
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			}, 1, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Send.Embeds[0].Image).NotTo(BeNil())
			Expect(result.Send.Embeds[0].Image.URL).To(Equal("https://example.com/thumb.jpg"))
			Expect(result.Send.Embeds[0].Description).To(ContainSubstring("> Video Title"))
			Expect(result.Send.Embeds[0].Description).To(ContainSubstring("> Video description here"))
		})

		It("processes embeds with image field", func() {
			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
				Embeds: []*discordgo.MessageEmbed{
					{
						Image: &discordgo.MessageEmbedImage{
							URL: "https://example.com/embed-image.jpg",
						},
					},
				},
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			}, 1, false)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Send.Embeds[0].Image).NotTo(BeNil())
			Expect(result.Send.Embeds[0].Image.URL).To(Equal("https://example.com/embed-image.jpg"))
		})

		It("processes embeds with video links", func() {
			message := &discordgo.Message{
				ID:        messageID,
				ChannelID: channelID,
				GuildID:   guildID,
				Author: &discordgo.User{
					ID:       authorID,
					Username: "testuser",
					Avatar:   "test",
				},
				Content: "Check out this video",
				Embeds: []*discordgo.MessageEmbed{
					{
						Video: &discordgo.MessageEmbedVideo{
							URL: "https://example.com/video.mp4",
						},
					},
				},
			}
			ch := &discordgo.Channel{Name: "general"}

			result, err := builder.Build(ch, message, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			}, 1, false)

			Expect(err).NotTo(HaveOccurred())
			fields := result.Send.Embeds[0].Fields
			Expect(fields).To(HaveLen(2))
			Expect(fields[1].Name).To(Equal("Embedded video"))
			Expect(fields[1].Value).To(Equal("[Click here](https://example.com/video.mp4)"))
			Expect(fields[1].Inline).To(BeTrue())
		})
	})

	Describe("UpdateFooter", func() {
		It("updates footer text with count", func() {
			embed := &discordgo.MessageEmbed{
				Footer: &discordgo.MessageEmbedFooter{
					Text:    "old text",
					IconURL: "old url",
				},
			}

			builder.UpdateFooter(embed, 10, false, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			})

			Expect(embed.Footer.Text).To(Equal("10"))
			Expect(embed.Footer.IconURL).To(ContainSubstring("2b50"))
		})

		It("includes self-star in footer text", func() {
			embed := &discordgo.MessageEmbed{
				Footer: &discordgo.MessageEmbedFooter{},
			}

			builder.UpdateFooter(embed, 3, true, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			})

			Expect(embed.Footer.Text).To(ContainSubstring("self-starred"))
		})

		It("creates footer if nil", func() {
			embed := &discordgo.MessageEmbed{}

			builder.UpdateFooter(embed, 7, false, &discordgo.MessageReactions{
				Emoji: &discordgo.Emoji{ID: "", Name: "\u2b50"},
			})

			Expect(embed.Footer).NotTo(BeNil())
			Expect(embed.Footer.Text).To(Equal("7"))
		})
	})
})
