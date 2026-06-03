package logging

import (
	"log/slog"
	"os"
	"strings"
)



var Logger = slog.New(
	slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}),
)

func Init(level string, service string, version string)  {
	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)

	Logger = slog.New(handler).With(
		"service", service,
		"version", version,
	)

	slog.SetDefault(Logger)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo

	}
}