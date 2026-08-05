package embed

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/embeds"
	"github.com/bwmarrin/discordgo"
)

var _ = Describe("extract", func() {
	var eb *embeds.Builder

	BeforeEach(func() {
		eb = embeds.NewBuilder()
	})

	Describe("single media type", func() {
		It("processes attachment image URLs", func() {
			message := &discordgo.Message{
				Attachments: []*discordgo.MessageAttachment{
					{URL: "https://example.com/image.png"},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			Expect(result.err).NotTo(HaveOccurred())
			Expect(eb.Finalize().Image).NotTo(BeNil())
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/image.png"))
		})

		It("processes embed thumbnails", func() {
			message := &discordgo.Message{
				Embeds: []*discordgo.MessageEmbed{
					{
						Thumbnail: &discordgo.MessageEmbedThumbnail{
							ProxyURL: "https://example.com/thumb.jpg",
						},
					},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			Expect(result.err).NotTo(HaveOccurred())
			Expect(eb.Finalize().Image).NotTo(BeNil())
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/thumb.jpg"))
		})

		It("processes embed images", func() {
			message := &discordgo.Message{
				Embeds: []*discordgo.MessageEmbed{
					{
						Image: &discordgo.MessageEmbedImage{
							URL: "https://example.com/embed-image.jpg",
						},
					},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			Expect(result.err).NotTo(HaveOccurred())
			Expect(eb.Finalize().Image).NotTo(BeNil())
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/embed-image.jpg"))
		})

		It("processes URL images from message content", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/photo.png",
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			Expect(result.err).NotTo(HaveOccurred())
			Expect(eb.Finalize().Image).NotTo(BeNil())
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/photo.png"))
		})

		It("processes URL video from message content", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/video.mp4",
			}

			result := extract(eb, message)

			Expect(result.err).NotTo(HaveOccurred())
		})

		It("processes stickers", func() {
			message := &discordgo.Message{
				StickerItems: []*discordgo.StickerItem{
					{ID: "123", FormatType: discordgo.StickerFormatTypePNG},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			Expect(result.err).NotTo(HaveOccurred())
			Expect(eb.Finalize().Image).NotTo(BeNil())
			Expect(eb.Finalize().Image.URL).To(Equal("https://cdn.discordapp.com/stickers/123.png"))
		})

		It("processes GIF stickers", func() {
			message := &discordgo.Message{
				StickerItems: []*discordgo.StickerItem{
					{ID: "456", FormatType: discordgo.StickerFormatTypeGIF},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			Expect(eb.Finalize().Image.URL).To(Equal("https://cdn.discordapp.com/stickers/456.gif"))
		})

		It("returns empty result for no media", func() {
			message := &discordgo.Message{}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeFalse())
			Expect(result.file).To(BeNil())
			Expect(result.modify).To(BeNil())
			Expect(result.err).NotTo(HaveOccurred())
		})
	})

	Describe("priority rules", func() {
		It("prefers embeds over attachments", func() {
			message := &discordgo.Message{
				Attachments: []*discordgo.MessageAttachment{
					{URL: "https://example.com/attachment.png"},
				},
				Embeds: []*discordgo.MessageEmbed{
					{
						Thumbnail: &discordgo.MessageEmbedThumbnail{
							ProxyURL: "https://example.com/thumb.jpg",
						},
					},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			// Embed thumbnail should win over attachment image
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/thumb.jpg"))
		})

		It("prefers embeds over URLs", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/photo.png",
				Embeds: []*discordgo.MessageEmbed{
					{
						Thumbnail: &discordgo.MessageEmbedThumbnail{
							ProxyURL: "https://example.com/thumb.jpg",
						},
					},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			// Embed thumbnail should win over URL image
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/thumb.jpg"))
		})

		It("prefers URLs over stickers", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/photo.png",
				StickerItems: []*discordgo.StickerItem{
					{ID: "123", FormatType: discordgo.StickerFormatTypePNG},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			// URL image should win over sticker
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/photo.png"))
		})

		It("prefers embeds over stickers", func() {
			message := &discordgo.Message{
				StickerItems: []*discordgo.StickerItem{
					{ID: "123", FormatType: discordgo.StickerFormatTypePNG},
				},
				Embeds: []*discordgo.MessageEmbed{
					{
						Thumbnail: &discordgo.MessageEmbedThumbnail{
							ProxyURL: "https://example.com/thumb.jpg",
						},
					},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			// Embed thumbnail should win over sticker
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/thumb.jpg"))
		})

		It("prefers embeds over all other media types", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/photo.png",
				Attachments: []*discordgo.MessageAttachment{
					{URL: "https://example.com/attachment.png"},
				},
				Embeds: []*discordgo.MessageEmbed{
					{
						Thumbnail: &discordgo.MessageEmbedThumbnail{
							ProxyURL: "https://example.com/thumb.jpg",
						},
					},
				},
				StickerItems: []*discordgo.StickerItem{
					{ID: "123", FormatType: discordgo.StickerFormatTypePNG},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			// Embed should win over everything
			Expect(eb.Finalize().Image.URL).To(Equal("https://example.com/thumb.jpg"))
		})

		It("falls back to sticker when only sticker is present", func() {
			message := &discordgo.Message{
				StickerItems: []*discordgo.StickerItem{
					{ID: "sticker123", FormatType: discordgo.StickerFormatTypePNG},
				},
			}

			result := extract(eb, message)

			Expect(result.hasImage).To(BeTrue())
			Expect(eb.Finalize().Image.URL).To(Equal("https://cdn.discordapp.com/stickers/sticker123.png"))
		})
	})

	Describe("content modification", func() {
		It("removes URL from content when processing URL image", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/photo.png",
			}

			result := extract(eb, message)

			Expect(result.modify).NotTo(BeNil())
			modified := result.modify("Check this: https://example.com/photo.png")
			Expect(modified).To(Equal("Check this: "))
		})

		It("removes URL from content when processing URL video", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/video.mp4",
			}

			result := extract(eb, message)

			Expect(result.modify).NotTo(BeNil())
			modified := result.modify("Check this: https://example.com/video.mp4")
			Expect(modified).To(Equal("Check this: "))
		})

		It("removes attachment URL from content", func() {
			message := &discordgo.Message{
				Content:     "Check this: https://example.com/image.png",
				Attachments: []*discordgo.MessageAttachment{{URL: "https://example.com/image.png"}},
			}

			result := extract(eb, message)

			Expect(result.modify).NotTo(BeNil())
			modified := result.modify("Check this: https://example.com/image.png")
			Expect(modified).To(Equal("Check this: "))
		})

		It("chains modify functions from multiple media types", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/photo.png",
				Embeds: []*discordgo.MessageEmbed{
					{
						Thumbnail: &discordgo.MessageEmbedThumbnail{
							ProxyURL: "https://example.com/thumb.jpg",
						},
						Description: "Embed description",
						Title:       "Embed Title",
					},
				},
			}

			result := extract(eb, message)

			Expect(result.modify).NotTo(BeNil())

			// The chained modify should:
			// 1. NOT remove URL from content (URL image was overwritten by embed thumbnail)
			// 2. Append embed description
			originalContent := "Check this: https://example.com/photo.png"
			modified := result.modify(originalContent)
			Expect(modified).To(ContainSubstring("> Embed Title"))
			Expect(modified).To(ContainSubstring("> Embed description"))

			// URL image was overwritten, so URL should remain in content
			Expect(modified).To(ContainSubstring("https://example.com/photo.png"))
		})

		It("does not remove URL from content when image is overwritten by embed", func() {
			message := &discordgo.Message{
				Content: "Check this: https://example.com/photo.png",
				Embeds: []*discordgo.MessageEmbed{
					{
						Thumbnail: &discordgo.MessageEmbedThumbnail{
							ProxyURL: "https://example.com/thumb.jpg",
						},
					},
				},
			}

			result := extract(eb, message)

			Expect(result.modify).NotTo(BeNil())
			modified := result.modify("Check this: https://example.com/photo.png")

			// URL image was overwritten by embed thumbnail, so URL should remain
			Expect(modified).To(ContainSubstring("https://example.com/photo.png"))
		})
	})

	Describe("field validation", func() {
		It("adds attachment fields for additional attachments", func() {
			message := &discordgo.Message{
				Attachments: []*discordgo.MessageAttachment{
					{URL: "https://example.com/image1.png"},
					{URL: "https://example.com/image2.png"},
					{URL: "https://example.com/image3.png"},
				},
			}

			extract(eb, message)

			fields := eb.Finalize().Fields
			Expect(fields).To(HaveLen(2))
			Expect(fields[0].Name).To(Equal("Attachment 2"))
			Expect(fields[0].Value).To(Equal("[Click here](https://example.com/image2.png)"))
			Expect(fields[1].Name).To(Equal("Attachment 3"))
			Expect(fields[1].Value).To(Equal("[Click here](https://example.com/image3.png)"))
		})

		It("adds embed video field when present", func() {
			message := &discordgo.Message{
				Embeds: []*discordgo.MessageEmbed{
					{
						Video: &discordgo.MessageEmbedVideo{
							URL: "https://example.com/video.mp4",
						},
					},
				},
			}

			result := extract(eb, message)

			Expect(result.err).NotTo(HaveOccurred())
			fields := eb.Finalize().Fields
			Expect(fields).To(HaveLen(1))
			Expect(fields[0].Name).To(Equal("Embedded video"))
			Expect(fields[0].Value).To(Equal("[Click here](https://example.com/video.mp4)"))
		})
	})
})
