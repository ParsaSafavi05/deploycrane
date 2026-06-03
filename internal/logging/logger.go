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

		handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: parseLevel(level),
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					// cleaner timestamp format
					t := a.Value.Time()
					a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05.000"))
				}
				return a
			},
		})
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

// Wrapper functions

func Info(msg string, args ...any) {
	Logger.Info(msg, args...)
}

func Error(msg string, args ...any) {
	Logger.Error(msg, args...)
}

func Debug(msg string, args ...any) {
	Logger.Debug(msg, args...)
}

func Warn(msg string, args ...any) {
	Logger.Warn(msg, args...)
}

func Fatal(msg string, args ...any) {
	Logger.Error(msg, args...)
	os.Exit(1)
}