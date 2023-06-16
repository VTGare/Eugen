package middlewares

import (
	"context"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/api/cmdroute"
	"github.com/diamondburned/arikawa/v3/discord"
	"go.uber.org/zap"
)

func CommandLog(logger *zap.SugaredLogger) cmdroute.Middleware {
	return func(next cmdroute.InteractionHandler) cmdroute.InteractionHandler {
		mw := func(ctx context.Context, ie *discord.InteractionEvent) *api.InteractionResponse {
			if ie.Data.InteractionType() != discord.CommandInteractionType {
				return next.HandleInteraction(ctx, ie)
			}

			cmd := ie.Data.(*discord.CommandInteraction)

			logger.With(
				"sender", ie.SenderID(),
				"guild_id", ie.GuildID,
				"channel_id", ie.ChannelID,
				"command", cmd.Name,
				"options", cmd.Options,
			).Info("executing a command")

			return next.HandleInteraction(ctx, ie)
		}

		return cmdroute.InteractionHandlerFunc(mw)
	}
}
