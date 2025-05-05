package logctx

import (
	"context"

	"github.com/rs/zerolog"
)

func With(ctx context.Context, logger zerolog.Logger) context.Context {
	return logger.WithContext(ctx)
}

func From(ctx context.Context) zerolog.Logger {
	return zerolog.Ctx(ctx)
}
