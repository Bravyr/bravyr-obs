// Package log provides a zerolog-based structured logger with Seq CLEF shipping.
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

	// SeqURL is the base URL of the Seq server (e.g. "https://seq.example.com:5341").
	// Leave empty to disable Seq shipping.
	SeqURL string

	// SeqAPIKey is the optional Seq ingest API key.
	SeqAPIKey string

	// ServiceName is attached as the "service" field on every log event.
	ServiceName string

	// DevMode enables human-readable console output. Must not be true in production.
	DevMode bool
}

// Logger wraps zerolog.Logger with a lifecycle method for shutting down the
// async Seq writer. All level methods return *zerolog.Event so callers can
// chain fields in the standard zerolog style:
//
//	logger.Info().Str("key", "value").Msg("something happened")
type Logger struct {
	zl  zerolog.Logger
	seq *seqWriter // nil when Seq shipping is disabled
}

// New constructs a Logger from cfg. Returns an error if the level string is
// unrecognised (valid values: debug, info, warn, error, fatal; empty → info).
func New(cfg Config) (*Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	var writers []io.Writer

	if cfg.DevMode {
		// Pretty console writer for local development. Zerolog's ConsoleWriter
		// formats each event as colourised, human-readable text.
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stdout})
	} else {
		// In non-dev mode, emit structured JSON to stderr so container runtimes
		// and systemd can still capture logs even if Seq is misconfigured.
		writers = append(writers, os.Stderr)
	}

	var sw *seqWriter
	if cfg.SeqURL != "" {
		sw = newSeqWriter(cfg.SeqURL, cfg.SeqAPIKey)
		writers = append(writers, sw)
	}

	var w io.Writer
	switch len(writers) {
	case 0:
		w = io.Discard
	case 1:
		w = writers[0]
	default:
		w = zerolog.MultiLevelWriter(writers...)
	}

	zl := zerolog.New(w).
		Level(level).
		With().
		Str("service", cfg.ServiceName).
		Timestamp().
		Logger()

	return &Logger{zl: zl, seq: sw}, nil
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
// writers and are flushed when the parent is shut down.
//
//	reqLog := logger.With().Str("request_id", id).Logger()
func (l *Logger) With() zerolog.Context { return l.zl.With() }

// Shutdown flushes the Seq writer if one is configured and waits for the
// background goroutine to finish. ctx is accepted for API symmetry but the
// underlying close is unconditional — Seq drain is always attempted.
func (l *Logger) Shutdown(_ context.Context) error {
	if l.seq != nil {
		return l.seq.Close()
	}
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
