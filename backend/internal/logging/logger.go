package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New creates a structured slog logger from level and format settings
// (JSON by default: time, level, msg and attributes). Secret attributes
// and secret-looking text are masked (RedactAttr); records logged with a
// request context carry request_id and organization_uuid.
func New(level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level), ReplaceAttr: RedactAttr}

	var handler slog.Handler
	if strings.EqualFold(format, "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(contextHandler{next: handler})
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
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
