// Package testutil provides shared setup for integration tests that need a real
// database. It solves a subtle problem: `go test ./...` runs different packages'
// tests in PARALLEL, and our store/experiment integration tests all point at the
// same TEST_DATABASE_URL. Two packages truncating the same tables at once corrupt
// each other's runs.
//
// Setup serializes cross-package DB access with a Postgres ADVISORY LOCK held on
// a dedicated connection for the duration of each test. Only one test anywhere
// holds the lock at a time, so truncation + work never interleave. Within a
// package tests already run serially, so this only ever blocks across packages.
package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/1morshed1/prompt-hub/server/internal/db"
	"github.com/1morshed1/prompt-hub/server/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// advisoryKey is an arbitrary fixed key shared by every integration test so they
// all contend on the same lock.
const advisoryKey = 918273645

// Setup connects to TEST_DATABASE_URL, applies migrations, takes the global test
// advisory lock, and truncates all tables so the test starts from a clean slate.
// It registers cleanup that releases the lock and closes the pool. If
// TEST_DATABASE_URL is unset, the test is skipped.
func Setup(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run integration tests")
	}
	ctx := context.Background()

	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}

	// Acquire a dedicated connection and hold the advisory lock on it. Holding
	// the same connection is required — advisory locks are session-scoped.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		t.Fatalf("acquire lock conn: %v", err)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryKey); err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("advisory lock: %v", err)
	}

	// Clean slate under the lock.
	if _, err := pool.Exec(ctx, `TRUNCATE prompts, prompt_versions, prompt_tags,
		experiments, experiment_results RESTART IDENTITY CASCADE`); err != nil {
		_, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryKey)
		conn.Release()
		pool.Close()
		t.Fatalf("truncate: %v", err)
	}

	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryKey)
		conn.Release()
		pool.Close()
	})
	return store.New(pool)
}

// Pool is occasionally handy for a test that needs raw access; most tests only
// need the returned *store.Store.
func Pool(s *store.Store) *pgxpool.Pool { return s.Pool() }
