// Package db owns the PostgreSQL connection pool and the migration runner.
//
// We use pgxpool (github.com/jackc/pgx/v5/pgxpool) directly rather than
// database/sql: pgx has better Postgres type support (JSONB, arrays, UUID) and
// its pool is purpose-built. The rest of the app talks to *pgxpool.Pool.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool and verifies it with a ping. It retries a few
// times because in local/dev setups the database container often starts a moment
// after the server does.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	var pool *pgxpool.Pool
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = pool.Ping(pingCtx)
			cancel()
			if err == nil {
				return pool, nil
			}
			pool.Close()
		}
		lastErr = err
		// Exponential-ish backoff: 0.5s, 1s, 2s, 4s.
		time.Sleep(time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond)
	}
	return nil, fmt.Errorf("connect to database after 5 attempts: %w", lastErr)
}
