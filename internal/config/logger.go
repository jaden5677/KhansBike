package config

import (
	"log/slog"
	"os"
)

// NewLogger builds the process-wide structured logger from configuration.
//
// Why here: the logger's handler and level are pure functions of LOG_FORMAT and
// LOG_LEVEL, so constructing it beside config keeps the wiring in one place and
// lets every binary obtain an identically-configured logger with one call. A
// JSON handler is used in production (machine-ingestible), a text handler in
// development (human-readable in a terminal).
func NewLogger(c *Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	var h slog.Handler
	if c.LogFormat == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(h)
}
