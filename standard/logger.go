package standard

import (
	"context"
	"log/slog"

	"github.com/go-sdk/core/logx"
	grpclogging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
)

func newLogger() grpclogging.Logger {
	return grpclogging.LoggerFunc(func(ctx context.Context, level grpclogging.Level, message string, fields ...any) {
		logger := logx.Ctx(ctx)
		event := logger.Info()
		switch {
		case level >= grpclogging.Level(slog.LevelError):
			event = logger.Error()
		case level >= grpclogging.Level(slog.LevelWarn):
			event = logger.Warn()
		case level <= grpclogging.Level(slog.LevelDebug):
			event = logger.Debug()
		}
		event.Fields(fields).Msg(message)
	})
}
