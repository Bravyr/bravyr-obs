package log

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// newTestLogger builds a Logger that writes JSON to buf, making assertions
// on structured output straightforward without relying on console formatting.
func newTestLogger(t *testing.T, buf *bytes.Buffer, cfg Config) *Logger {
	t.Helper()

	level, err := parseLevel(cfg.Level)
	if err != nil {
		t.Fatalf("parseLevel: %v", err)
	}

	zl := zerolog.New(buf).Level(level).With().
		Str("service", cfg.ServiceName).
		Timestamp().
		Logger()

	return &Logger{zl: zl}
}

func TestNew_devMode(t *testing.T) {
	// DevMode=true should produce a logger without error.
	l, err := New(Config{
		ServiceName: "test-svc",
		Level:       "info",
		DevMode:     true,
	})
	if err != nil {
		t.Fatalf("New() with DevMode=true returned error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil Logger")
	}
	_ = l.Shutdown(context.Background())
}

func TestNew_invalidLevel(t *testing.T) {
	_, err := New(Config{
		ServiceName: "test-svc",
		Level:       "banana",
	})
	if err == nil {
		t.Fatal("expected error for unrecognised level")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Fatalf("error should mention the bad level, got: %v", err)
	}
}

func TestNew_defaultLevel(t *testing.T) {
	// Empty Level should default to info without error.
	l, err := New(Config{ServiceName: "test-svc", Level: ""})
	if err != nil {
		t.Fatalf("New() with empty Level returned error: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil Logger")
	}
	_ = l.Shutdown(context.Background())
}

func TestLogger_levels(t *testing.T) {
	cases := []struct {
		name      string
		emit      func(l *Logger)
		wantLevel string
	}{
		{"debug", func(l *Logger) { l.Debug().Msg("d") }, "debug"},
		{"info", func(l *Logger) { l.Info().Msg("i") }, "info"},
		{"warn", func(l *Logger) { l.Warn().Msg("w") }, "warn"},
		{"error", func(l *Logger) { l.Error().Msg("e") }, "error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := newTestLogger(t, &buf, Config{ServiceName: "svc", Level: "debug"})

			tc.emit(l)

			var event map[string]any
			if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
				t.Fatalf("json.Unmarshal failed: %v (output: %q)", err, buf.String())
			}
			got, _ := event["level"].(string)
			if got != tc.wantLevel {
				t.Fatalf("expected level %q, got %q", tc.wantLevel, got)
			}
		})
	}
}

func TestLogger_with(t *testing.T) {
	var buf bytes.Buffer
	base := newTestLogger(t, &buf, Config{ServiceName: "svc", Level: "info"})

	sub := base.With().Str("request_id", "abc-123").Logger()
	sub.Info().Msg("scoped")

	var event map[string]any
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	rid, _ := event["request_id"].(string)
	if rid != "abc-123" {
		t.Fatalf("expected request_id=abc-123, got %q", rid)
	}
}

func TestShutdown_noop(t *testing.T) {
	l, err := New(Config{ServiceName: "test-svc", Level: "info"})
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	// Shutdown is a no-op — must not panic or return an error.
	if err := l.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() returned unexpected error: %v", err)
	}
}
