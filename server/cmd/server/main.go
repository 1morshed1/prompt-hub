// Command server is the PromptHub API server entrypoint.
//
// Boot sequence: load config → connect to Postgres → apply migrations → start
// the HTTP server. The full HTTP wiring is added in a later step; for now this
// establishes the boot path (config + db + migrations) so each layer can be
// verified as it's built.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/1morshed1/prompt-hub/server/internal/config"
	"github.com/1morshed1/prompt-hub/server/internal/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	fmt.Println("connected to database")

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	fmt.Println("migrations up to date")

	// HTTP server wiring is added in the API step. For now, boot succeeds and
	// exits cleanly, which is enough to verify config + db + migrations.
	fmt.Println("boot ok (HTTP server wired up in a later step)")
	return nil
}
