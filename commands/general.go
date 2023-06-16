package commands

import (
	"context"
	"time"

	"github.com/VTGare/Eugen/arikawautils/embeds"
	"github.com/VTGare/Eugen/bot"
	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/api/cmdroute"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

func ping(b *bot.Bot) (api.CreateCommandData, cmdroute.CommandHandlerFunc) {
	cmd := api.CreateCommandData{
		Name:        "ping",
		Description: "Get the bot's response time",
		Type:        discord.ChatInputCommand,
	}

	return cmd, func(ctx context.Context, data cmdroute.CommandData) *api.InteractionResponseData {
		latency := b.State.Gateway().Latency().Round(time.Millisecond).String()

		eb := embeds.NewBuilder()
		eb.Title("🏓 Pong!").AddField("Latency", latency)

		return &api.InteractionResponseData{
			Embeds: &[]discord.Embed{
				eb.Build(),
			},
		}
	}
}

func ignore(b *bot.Bot) (api.CreateCommandData, cmdroute.CommandHandlerFunc) {
	cmd := api.CreateCommandData{
		Name:                     "ignore",
		Description:              "Ignore channel or user",
		DefaultMemberPermissions: discord.NewPermissions(discord.PermissionManageGuild),
		Options: discord.CommandOptions{
			discord.NewUserOption("user", "Ignore a user", false),
			discord.NewChannelOption("channel", "Ignore a channel or category", false),
		},
	}

	return cmd, func(ctx context.Context, data cmdroute.CommandData) *api.InteractionResponseData {
		var (
			user    = data.Options.Find("user").String()
			channel = data.Options.Find("channel").String()
		)

		b.Log.With("user", user, "channel", channel).Info("received command")
		return &api.InteractionResponseData{Content: option.NewNullableString("Received.")}
	}
}
