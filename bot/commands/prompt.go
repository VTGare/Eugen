package commands

import (
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
)

// createPrompt sends an embed and blocks until the same author replies with
// a message, or times out after 2 minutes.
func createPrompt(s *discordgo.Session, m *discordgo.MessageCreate, embed *discordgo.MessageEmbed) string {
	prompt, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	if err != nil {
		slog.Warn("creating prompt", "err", err)
		return ""
	}

	var msg *discordgo.MessageCreate
	for {
		select {
		case msg = <-nextMessageCreate(s):
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