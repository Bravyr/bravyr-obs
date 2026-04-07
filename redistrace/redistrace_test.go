package redistrace

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestOptions_defaultsNoDBStatement(t *testing.T) {
	var cfg options
	if cfg.dbStatement {
		t.Fatal("expected dbStatement to default to false")
	}
	if cfg.metrics {
		t.Fatal("expected metrics to default to false")
	}
}

func TestOptions_withDBStatement(t *testing.T) {
	var cfg options
	WithDBStatement()(&cfg)
	if !cfg.dbStatement {
		t.Fatal("expected dbStatement to be true")
	}
}

func TestOptions_withMetrics(t *testing.T) {
	var cfg options
	WithMetrics()(&cfg)
	if !cfg.metrics {
		t.Fatal("expected metrics to be true")
	}
}

func TestInstrument_defaultOptions(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := Instrument(rdb); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstrument_withAllOptions(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := Instrument(rdb, WithDBStatement(), WithMetrics()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
