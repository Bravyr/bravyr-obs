package health

import (
	"context"
	"errors"
	"testing"
)

// mockPinger implements PingChecker for testing PgxCheck.
type mockPinger struct{ err error }

func (m *mockPinger) Ping(_ context.Context) error { return m.err }

// mockRedisResult implements RedisResult for testing RedisCheck.
type mockRedisResult struct{ err error }

func (m *mockRedisResult) Err() error { return m.err }

// mockRedisClient implements RedisChecker for testing RedisCheck.
type mockRedisClient struct{ result *mockRedisResult }

func (m *mockRedisClient) Ping(_ context.Context) RedisResult { return m.result }

func TestPgxCheck_healthy(t *testing.T) {
	fn := PgxCheck(&mockPinger{err: nil})
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestPgxCheck_unhealthy(t *testing.T) {
	want := errors.New("down")
	fn := PgxCheck(&mockPinger{err: want})
	if err := fn(context.Background()); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestRedisCheck_healthy(t *testing.T) {
	fn := RedisCheck(&mockRedisClient{result: &mockRedisResult{err: nil}})
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRedisCheck_unhealthy(t *testing.T) {
	want := errors.New("redis unavailable")
	fn := RedisCheck(&mockRedisClient{result: &mockRedisResult{err: want}})
	if err := fn(context.Background()); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

// TestPgxCheck_respectsContext verifies that when the caller supplies a
// cancelled context, the check propagates cancellation to the underlying
// Ping call rather than executing against a dead context silently.
func TestPgxCheck_respectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before calling the check

	// A real driver would return ctx.Err(); our mock also does so here to
	// verify the context is forwarded correctly.
	fn := PgxCheck(&ctxAwarePinger{})
	err := fn(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ctxAwarePinger returns ctx.Err() so we can verify the context is forwarded.
type ctxAwarePinger struct{}

func (p *ctxAwarePinger) Ping(ctx context.Context) error { return ctx.Err() }

func TestRedisCheck_respectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fn := RedisCheck(&ctxAwareRedisClient{})
	err := fn(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// ctxAwareRedisClient returns ctx.Err() wrapped in a RedisResult.
type ctxAwareRedisClient struct{}
type ctxAwareRedisResult struct{ err error }

func (r *ctxAwareRedisResult) Err() error                          { return r.err }
func (c *ctxAwareRedisClient) Ping(ctx context.Context) RedisResult { return &ctxAwareRedisResult{err: ctx.Err()} }

func TestRedisCheck_nilResult(t *testing.T) {
	fn := RedisCheck(&nilRedisClient{})
	err := fn(context.Background())
	if err == nil {
		t.Fatal("expected error for nil result, got nil")
	}
}

// nilRedisClient returns nil from Ping to test the nil guard.
type nilRedisClient struct{}

func (c *nilRedisClient) Ping(_ context.Context) RedisResult { return nil }

// --- TemporalCheck tests ---

type mockTemporalChecker struct{ err error }

func (m *mockTemporalChecker) CheckHealth(_ context.Context) error { return m.err }

func TestTemporalCheck_healthy(t *testing.T) {
	fn := TemporalCheck(&mockTemporalChecker{err: nil})
	if err := fn(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestTemporalCheck_unhealthy(t *testing.T) {
	want := errors.New("temporal unavailable")
	fn := TemporalCheck(&mockTemporalChecker{err: want})
	if err := fn(context.Background()); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestTemporalCheck_respectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fn := TemporalCheck(&ctxAwareTemporalChecker{})
	if err := fn(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type ctxAwareTemporalChecker struct{}

func (c *ctxAwareTemporalChecker) CheckHealth(ctx context.Context) error { return ctx.Err() }
