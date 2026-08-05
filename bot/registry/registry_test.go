package registry_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/VTGare/Eugen/bot/registry"
	"github.com/bwmarrin/discordgo"
)

var _ = Describe("Registry", func() {
	Describe("New", func() {
		It("returns an empty registry", func() {
			r := registry.New()
			Expect(r.Groups()).To(BeEmpty())
		})
	})

	Describe("Add", func() {
		var r *registry.Registry

		BeforeEach(func() {
			r = registry.New()
		})

		It("adds a command group", func() {
			g := &registry.CommandGroup{
				Name:        "basic",
				Description: "General commands",
				IsVisible:   true,
				Commands:    map[string]*registry.Command{},
			}
			r.Add("basic", g)

			Expect(r.Groups()).To(HaveLen(1))
			Expect(r.Group("basic")).NotTo(BeNil())
		})

		It("initializes nil Commands map", func() {
			g := &registry.CommandGroup{
				Name:        "test",
				Description: "test",
			}
			r.Add("test", g)

			Expect(r.Group("test").Commands).NotTo(BeNil())
		})

		It("overwrites existing group with same name", func() {
			g1 := &registry.CommandGroup{Name: "g1", Commands: map[string]*registry.Command{}}
			g2 := &registry.CommandGroup{Name: "g2", Commands: map[string]*registry.Command{}}
			r.Add("dup", g1)
			r.Add("dup", g2)

			Expect(r.Groups()).To(HaveLen(1))
			Expect(r.Group("dup").Name).To(Equal("g2"))
		})
	})

	Describe("Get", func() {
		var r *registry.Registry

		BeforeEach(func() {
			r = registry.New()
			g := &registry.CommandGroup{
				Name:        "basic",
				Description: "",
				Commands: map[string]*registry.Command{
					"ping": {Name: "ping", Description: "Check bot"},
					"help": {Name: "help", Description: "Show help"},
				},
			}
			r.Add("basic", g)
		})

		It("returns the command for a given name", func() {
			cmd := r.Get("ping")
			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Name).To(Equal("ping"))
		})

		It("returns nil for unknown command", func() {
			cmd := r.Get("nonexistent")
			Expect(cmd).To(BeNil())
		})

		It("finds commands across multiple groups", func() {
			g2 := &registry.CommandGroup{
				Name:     "admin",
				Commands: map[string]*registry.Command{"ban": {Name: "ban"}},
			}
			r.Add("admin", g2)

			cmd := r.Get("ban")
			Expect(cmd).NotTo(BeNil())
			Expect(cmd.Name).To(Equal("ban"))
		})
	})

	Describe("Group", func() {
		var r *registry.Registry

		BeforeEach(func() {
			r = registry.New()
		})

		It("returns the group by name", func() {
			g := &registry.CommandGroup{Name: "basic", Commands: map[string]*registry.Command{}}
			r.Add("basic", g)

			Expect(r.Group("basic")).To(Equal(g))
		})

		It("returns nil for unknown group", func() {
			Expect(r.Group("unknown")).To(BeNil())
		})
	})

	Describe("Groups", func() {
		It("returns all registered groups", func() {
			r := registry.New()
			g1 := &registry.CommandGroup{Name: "g1", Commands: map[string]*registry.Command{}}
			g2 := &registry.CommandGroup{Name: "g2", Commands: map[string]*registry.Command{}}
			r.Add("g1", g1)
			r.Add("g2", g2)

			groups := r.Groups()
			Expect(groups).To(HaveLen(2))
		})

		It("returns empty slice for empty registry", func() {
			r := registry.New()
			Expect(r.Groups()).To(BeEmpty())
		})
	})
})

var _ = Describe("Command", func() {
	Describe("CreateHelp", func() {
		It("replaces {prefix} placeholder in description", func() {
			cmd := &registry.Command{
				Name:        "test",
				Description: "Use {prefix}test to do something",
			}
			Expect(cmd.CreateHelp("e!")).To(Equal("Use e!test to do something"))
		})

		It("leaves description unchanged if no placeholder", func() {
			cmd := &registry.Command{
				Name:        "test",
				Description: "No placeholder here",
			}
			Expect(cmd.CreateHelp("e!")).To(Equal("No placeholder here"))
		})
	})

	Describe("CreateExtendedHelp", func() {
		It("returns empty slice when Help is nil", func() {
			cmd := &registry.Command{Name: "test"}
			Expect(cmd.CreateExtendedHelp("e!")).To(BeEmpty())
		})

		It("returns empty slice when ExtendedHelp is nil", func() {
			cmd := &registry.Command{
				Name: "test",
				Help: &registry.HelpSettings{IsVisible: true},
			}
			Expect(cmd.CreateExtendedHelp("e!")).To(BeEmpty())
		})

		It("returns empty slice when ExtendedHelp is empty", func() {
			cmd := &registry.Command{
				Name: "test",
				Help: &registry.HelpSettings{
					IsVisible:    true,
					ExtendedHelp: []*discordgo.MessageEmbedField{},
				},
			}
			Expect(cmd.CreateExtendedHelp("e!")).To(BeEmpty())
		})

		It("replaces {prefix} in extended help values", func() {
			cmd := &registry.Command{
				Name: "test",
				Help: &registry.HelpSettings{
					ExtendedHelp: []*discordgo.MessageEmbedField{
						{Name: "Usage", Value: "``{prefix}test``", Inline: false},
					},
				},
			}
			result := cmd.CreateExtendedHelp("e!")
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("Usage"))
			Expect(result[0].Value).To(Equal("``e!test``"))
			Expect(result[0].Inline).To(BeFalse())
		})

		It("adds guild-only permissions field when GuildOnly is true", func() {
			cmd := &registry.Command{
				Name:      "test",
				GuildOnly: true,
				Help: &registry.HelpSettings{
					ExtendedHelp: []*discordgo.MessageEmbedField{
						{Name: "Usage", Value: "``{prefix}test``"},
					},
				},
			}
			result := cmd.CreateExtendedHelp("!")

			Expect(result).To(HaveLen(2))
			Expect(result[0].Name).To(Equal("Permissions"))
			Expect(result[0].Value).To(ContainSubstring("guild-only"))
			Expect(result[1].Name).To(Equal("Usage"))
		})

		It("does not add permissions field when GuildOnly is false", func() {
			cmd := &registry.Command{
				Name:      "test",
				GuildOnly: false,
				Help: &registry.HelpSettings{
					ExtendedHelp: []*discordgo.MessageEmbedField{
						{Name: "Usage", Value: "``{prefix}test``"},
					},
				},
			}
			result := cmd.CreateExtendedHelp("!")
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("Usage"))
		})

		It("handles multiple extended help fields", func() {
			cmd := &registry.Command{
				Name: "test",
				Help: &registry.HelpSettings{
					ExtendedHelp: []*discordgo.MessageEmbedField{
						{Name: "Usage", Value: "``{prefix}test``"},
						{Name: "Examples", Value: "``{prefix}test foo``"},
						{Name: "Description", Value: "{prefix}test does things"},
					},
				},
			}
			result := cmd.CreateExtendedHelp("e!")
			Expect(result).To(HaveLen(3))
			// Verify all prefix placeholders were replaced.
			for _, f := range result {
				Expect(f.Value).To(ContainSubstring("e!test"))
				Expect(f.Value).NotTo(ContainSubstring("{prefix}"))
			}
		})
	})
})
