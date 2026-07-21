package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
)

func Run(ctx context.Context, dbPath string, logPath string) error {
	db, err := database.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	if migrationsErr := database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath); migrationsErr != nil {
		return fmt.Errorf("failed to run migrations: %w", migrationsErr)
	}

	version, dirty, err := database.GetCurrentVersion(db, database.MigrationsFS, database.MigrationsPath)
	if err != nil {
		log.Printf("Warning: Could not get schema version: %v", err)
	} else {
		log.Printf("Database schema version: %d (dirty: %v)", version, dirty)
		if dirty {
			log.Fatal("Database is in a dirty state. Manual intervention required.")
		}
	}

	pageViews := make(chan PageView, 100)

	// Draining pageViews and running a cleanup already in flight at shutdown
	// must not be aborted by the same cancellation that signals shutdown, so
	// DB writes use a context that keeps values but drops the cancel signal.
	dbCtx := context.WithoutCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		processPageviews(dbCtx, db, pageViews)
	}()
	go func() {
		defer wg.Done()
		runPeriodicCleanup(ctx, dbCtx, db, time.NewTicker(12*time.Hour))
	}()

	// tailLog blocks until ctx is canceled (e.g. by a SIGINT/SIGTERM wired
	// in by cmd.Execute), at which point exec.CommandContext kills the
	// "tail -f" child and unblocks the scanner loop below.
	tailArgs := []string{"-f", logPath}
	tailLog(ctx, tailArgs, pageViews)

	log.Println("Shutdown signal received, stopping...")
	close(pageViews)
	wg.Wait()

	return nil
}

// runPeriodicCleanup runs performAllCleanups on a timer until shutdown is
// canceled. dbCtx (not shutdown) is used for the cleanup queries themselves,
// so a cleanup already running when shutdown fires can still complete.
func runPeriodicCleanup(shutdown context.Context, dbCtx context.Context, db *sql.DB, ticker *time.Ticker) {
	performAllCleanups(dbCtx, db)

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			performAllCleanups(dbCtx, db)
		case <-shutdown.Done():
			log.Println("Cleanup goroutine shutting down...")
			return
		}
	}
}
