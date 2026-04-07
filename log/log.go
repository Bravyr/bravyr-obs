// Package log provides a zerolog-based structured logger that writes JSON to
// stdout. Logs are collected from container stdout by Promtail and shipped to
// Loki for aggregation.
package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// Config holds logger construction options. It mirrors the subset of
// config.Config that is relevant to logging so the log package remains
// importable on its own without pulling in the root config.
type Config struct {
	// Level is one of "debug", "info", "warn", "error", "fatal".
	// Empty string defaults to "info".
	Level string

	// ServiceName is attached as the "service" field on every log event.
	ServiceName string

	// DevMode enables human-readable console output. Must not be true in production.
	DevMode bool
}

// Logger wraps zerolog.Logger with a no-op Shutdown method for lifecycle
// symmetry with other observability providers. All level methods return
// *zerolog.Event so callers can chain fields in the standard zerolog style:
//
//	logger.Info().Str("key", "value").Msg("something happened")
type Logger struct {
	zl zerolog.Logger
}

// New constructs a Logger from cfg. Returns an error if the level string is
// unrecognised (valid values: debug, info, warn, error, fatal; empty → info).
func New(cfg Config) (*Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	var w io.Writer
	if cfg.DevMode {
		// Pretty console writer for local development. Zerolog's ConsoleWriter
		// formats each event as colourised, human-readable text.
		w = zerolog.ConsoleWriter{Out: os.Stdout}
	} else {
		// Structured JSON to stdout so Promtail can scrape logs from container
		// stdout and forward them to Loki.
		w = os.Stdout
	}

	zl := zerolog.New(w).
		Level(level).
		With().
		Str("service", cfg.ServiceName).
		Timestamp().
		Logger()

	return &Logger{zl: zl}, nil
}

// Debug returns a *zerolog.Event for a debug-level message.
func (l *Logger) Debug() *zerolog.Event { return l.zl.Debug() }

// Info returns a *zerolog.Event for an info-level message.
func (l *Logger) Info() *zerolog.Event { return l.zl.Info() }

// Warn returns a *zerolog.Event for a warning-level message.
func (l *Logger) Warn() *zerolog.Event { return l.zl.Warn() }

// Error returns a *zerolog.Event for an error-level message.
func (l *Logger) Error() *zerolog.Event { return l.zl.Error() }

// Fatal returns a *zerolog.Event for a fatal-level message. Calling Msg() on
// this event will call os.Exit(1) after logging — use with caution.
func (l *Logger) Fatal() *zerolog.Event { return l.zl.Fatal() }

// With returns a zerolog.Context so callers can build a sub-logger with
// additional fixed fields. Note that the resulting sub-logger is a raw
// zerolog.Logger (not *Logger), so it does not carry Shutdown or typed
// level methods. This is intentional — sub-loggers share the parent's
// writer and are collected alongside it.
//
//	reqLog := logger.With().Str("request_id", id).Logger()
func (l *Logger) With() zerolog.Context { return l.zl.With() }

// Shutdown is a no-op kept for API symmetry with other observability
// providers. Logs are written synchronously to stdout; no flushing is needed.
func (l *Logger) Shutdown(_ context.Context) error {
	return nil
}

// parseLevel converts the human-readable level string to a zerolog.Level.
// An empty string is treated as info (a sensible production default).
func parseLevel(s string) (zerolog.Level, error) {
	if s == "" {
		return zerolog.InfoLevel, nil
	}

	switch strings.ToLower(s) {
	case "debug":
		return zerolog.DebugLevel, nil
	case "info":
		return zerolog.InfoLevel, nil
	case "warn":
		return zerolog.WarnLevel, nil
	case "error":
		return zerolog.ErrorLevel, nil
	case "fatal":
		return zerolog.FatalLevel, nil
	default:
		return zerolog.Disabled, fmt.Errorf("log: unrecognised level %q", s)
	}
}
