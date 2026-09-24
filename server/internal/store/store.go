// Package store is the data-access layer: every SQL query lives here, behind
// methods that return domain types from the model package. Handlers and services
// never write SQL themselves — they call the Store. This keeps persistence in one
// auditable place and makes the rest of the app testable against an interface.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Common sentinel errors the layers above translate into HTTP status codes.
var (
	// ErrNotFound is returned when a lookup matches no row.
	ErrNotFound = errors.New("not found")
	// ErrConflict is returned when a unique constraint is violated (e.g. a
	// duplicate prompt name, or a second running experiment for a prompt).
	ErrConflict = errors.New("conflict")
)

// Store wraps the connection pool and exposes typed data operations.
type Store struct {
	pool *pgxpool.Pool
}

// New builds a Store over an existing pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the underlying pool for the rare caller that needs it (e.g. an
// integration test truncating tables). Prefer the typed methods everywhere else.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// withTx runs fn inside a transaction, committing on success and rolling back on
// any error or panic. Centralizing this keeps every multi-statement operation
// (create-version, move-tag) correct without repeating begin/commit/rollback.
func (s *Store) withTx(ctx context.Context, fn func(tx pgx.Tx) error) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		} else if err != nil {
			_ = tx.Rollback(ctx)
		} else {
			err = tx.Commit(ctx)
		}
	}()
	return fn(tx)
}
