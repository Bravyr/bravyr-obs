package metrics

import (
	"context"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestInit(t *testing.T) {
	reg, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}
	if reg == nil {
		t.Fatal("Init() returned nil registry")
	}
	_ = reg.Shutdown(context.Background())
}

func TestInit_withPrefix(t *testing.T) {
	reg, err := Init(Config{Prefix: "myapp", })
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Collect descriptor names via Describe to verify registration regardless
	// of whether any observations have been made. Prometheus only emits
	// time-series in Gather() once they have been observed, but descriptors are
	// always present after registration.
	descCh := make(chan *prometheus.Desc, 32)
	reg.reg.Describe(descCh)
	close(descCh)

	names := make(map[string]bool)
	for d := range descCh {
		// d.String() format: Desc{fqName: "name", ...}
		s := d.String()
		for _, want := range []string{
			"myapp_http_request_duration_seconds",
			"myapp_http_requests_total",
			"myapp_http_active_requests",
		} {
			if strings.Contains(s, want) {
				names[want] = true
			}
		}
	}

	for _, want := range []string{
		"myapp_http_request_duration_seconds",
		"myapp_http_requests_total",
		"myapp_http_active_requests",
	} {
		if !names[want] {
			t.Errorf("expected descriptor for %q to be registered", want)
		}
	}
}

func TestInit_noPrefix(t *testing.T) {
	reg, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	descCh := make(chan *prometheus.Desc, 32)
	reg.reg.Describe(descCh)
	close(descCh)

	names := make(map[string]bool)
	for d := range descCh {
		s := d.String()
		for _, want := range []string{
			"http_request_duration_seconds",
			"http_requests_total",
			"http_active_requests",
		} {
			if strings.Contains(s, want) {
				names[want] = true
			}
		}
	}

	for _, want := range []string{
		"http_request_duration_seconds",
		"http_requests_total",
		"http_active_requests",
	} {
		if !names[want] {
			t.Errorf("expected descriptor for %q to be registered", want)
		}
	}
}

func TestNewCounter(t *testing.T) {
	reg, err := Init(Config{Prefix: "svc", })
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	counter, err := reg.NewCounter("orders_total", "total orders placed", []string{"status"})
	if err != nil {
		t.Fatalf("NewCounter() returned error: %v", err)
	}
	if counter == nil {
		t.Fatal("NewCounter() returned nil")
	}

	// Verify it can be used — increment without panic.
	counter.WithLabelValues("success").Inc()
}

func TestNewCounter_prefixApplied(t *testing.T) {
	reg, err := Init(Config{Prefix: "svc", })
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	_, err = reg.NewCounter("events_total", "total events", []string{})
	if err != nil {
		t.Fatalf("NewCounter() returned error: %v", err)
	}

	// Use Describe to verify the metric name without requiring observations.
	descCh := make(chan *prometheus.Desc, 32)
	reg.reg.Describe(descCh)
	close(descCh)

	found := false
	for d := range descCh {
		if strings.Contains(d.String(), "svc_events_total") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected descriptor for svc_events_total to be registered")
	}
}

func TestNewHistogram(t *testing.T) {
	reg, err := Init(Config{Prefix: "svc", })
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	hist, err := reg.NewHistogram(
		"db_query_duration_seconds",
		"database query duration",
		[]string{"table"},
		prometheus.DefBuckets,
	)
	if err != nil {
		t.Fatalf("NewHistogram() returned error: %v", err)
	}
	if hist == nil {
		t.Fatal("NewHistogram() returned nil")
	}

	// Verify it can be used — observe without panic.
	hist.WithLabelValues("users").Observe(0.042)
}

func TestNewCounter_duplicate(t *testing.T) {
	reg, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	_, err = reg.NewCounter("my_counter_total", "first registration", []string{})
	if err != nil {
		t.Fatalf("first NewCounter() returned error: %v", err)
	}

	_, err = reg.NewCounter("my_counter_total", "second registration", []string{})
	if err == nil {
		t.Fatal("expected error for duplicate metric name, got nil")
	}
}

func TestNewHistogram_duplicate(t *testing.T) {
	reg, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	_, err = reg.NewHistogram("my_hist", "first", []string{}, nil)
	if err != nil {
		t.Fatalf("first NewHistogram() returned error: %v", err)
	}

	_, err = reg.NewHistogram("my_hist", "second", []string{}, nil)
	if err == nil {
		t.Fatal("expected error for duplicate histogram name, got nil")
	}
}

func TestInit_invalidPrefix(t *testing.T) {
	_, err := Init(Config{Prefix: "my-app"})
	if err == nil {
		t.Fatal("expected error for invalid prefix with hyphen")
	}
}

func TestNormalizeMethod(t *testing.T) {
	if got := normalizeMethod("GET"); got != "GET" {
		t.Fatalf("expected GET, got %q", got)
	}
	if got := normalizeMethod("MADEUP"); got != "OTHER" {
		t.Fatalf("expected OTHER, got %q", got)
	}
}

func TestShutdown_noOp(t *testing.T) {
	reg, err := Init(Config{})
	if err != nil {
		t.Fatalf("Init() returned error: %v", err)
	}

	// Shutdown must not return an error and must not block.
	if err := reg.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}
}
