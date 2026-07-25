package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
)

func Run(ctx context.Context, dbPath string, logPath string) error {
	// "tail -F" retries indefinitely when the file is missing or
	// unreadable rather than exiting, so a bad --log-path would otherwise
	// leave the daemon polling forever with no visible error. Fail fast
	// here with a clear message instead.
	if err := checkLogFileReadable(logPath); err != nil {
		return err
	}

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
	// "tail -F" child and unblocks the scanner loop below. A non-nil
	// return instead means tail exited on its own (e.g. missing file,
	// permission denied), which must reach the caller as a real failure.
	tailErr := tailLog(ctx, buildTailArgs(logPath), pageViews)
	if tailErr != nil {
		log.Printf("Log tailing stopped: %v", tailErr)
	} else {
		log.Println("Shutdown signal received, stopping...")
	}

	close(pageViews)
	wg.Wait()

	if tailErr != nil {
		return fmt.Errorf("tailing log: %w", tailErr)
	}
	return nil
}

// checkLogFileReadable returns a wrapped, actionable error (unwrappable via
// errors.Is against fs.ErrNotExist / fs.ErrPermission) if logPath can't be
// opened for reading.
func checkLogFileReadable(logPath string) error {
	logFile, err := os.Open(logPath) //nolint:gosec // path is an operator-provided flag, not user input
	if err != nil {
		return fmt.Errorf("opening log file %q: %w", logPath, err)
	}
	if err := logFile.Close(); err != nil {
		return fmt.Errorf("closing log file %q after readability check: %w", logPath, err)
	}
	return nil
}

// buildTailArgs builds the argument list for the "tail" child that follows the
// access log. The leading "-n 0" is load-bearing: without it, tail defaults to
// "-n 10" and replays the last 10 lines already in the file before following
// new appends. Since ingest has no offset/watermark and treats every emitted
// line as a fresh page view, that replay makes every daemon restart (systemctl
// restart, crash respawn, host reboot, system update) silently re-count up to
// the last 10 lines into hourly_stats/hourly_status_codes/hourly_referrers.
// "-n 0" starts following at end-of-file, so a restart against an unchanged log
// records nothing. "-F" (not "-f") keeps following across log rotation.
func buildTailArgs(logPath string) []string {
	return []string{"-n", "0", "-F", logPath}
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
