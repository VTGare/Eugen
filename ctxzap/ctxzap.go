package ctxzap

import (
	"context"

	"go.uber.org/zap"
)

type loggerKeyType int

const loggerKey loggerKeyType = 0

func ToContext(ctx context.Context, log *zap.SugaredLogger) context.Context {
	return context.WithValue(ctx, loggerKey, log)
}

func Extract(ctx context.Context) *zap.SugaredLogger {
	return ctx.Value(loggerKey).(*zap.SugaredLogger)
}
