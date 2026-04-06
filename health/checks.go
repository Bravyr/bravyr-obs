// Package health provides lightweight health check primitives for HTTP services.
package health

import (
	"context"
	"errors"
)

// PingChecker is satisfied by *pgxpool.Pool, *pgx.Conn, and any other type
// that exposes a Ping(context.Context) error method. Declaring the interface
// here — rather than importing a driver package — keeps this library free of
// driver dependencies.
type PingChecker interface {
	Ping(ctx context.Context) error
}

// PgxCheck returns a CheckFunc that pings a Postgres-compatible connection.
// It works with *pgxpool.Pool, *pgx.Conn, or any type satisfying PingChecker.
func PgxCheck(p PingChecker) CheckFunc {
	return func(ctx context.Context) error {
		return p.Ping(ctx)
	}
}

// RedisResult is satisfied by the *redis.StatusCmd returned by
// (*redis.Client).Ping. Declaring the interface here avoids importing the
// go-redis package.
type RedisResult interface {
	Err() error
}

// RedisChecker is satisfied by *redis.Client, *redis.ClusterClient, and other
// Redis client types that expose a Ping method returning a result with Err().
type RedisChecker interface {
	Ping(ctx context.Context) RedisResult
}

// RedisCheck returns a CheckFunc that pings a Redis-compatible client.
// It works with any type satisfying RedisChecker.
func RedisCheck(c RedisChecker) CheckFunc {
	return func(ctx context.Context) error {
		res := c.Ping(ctx)
		if res == nil {
			return errors.New("redis ping returned nil result")
		}
		return res.Err()
	}
}
