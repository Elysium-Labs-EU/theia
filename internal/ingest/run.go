package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go processPageviews(ctx, db, pageViews)
	go runPeriodicCleanup(ctx, db, time.NewTicker(12*time.Hour), sigChan)

	go handleShutdownSignal(sigChan, pageViews)

	tailArgs := []string{"-f", logPath}
	tailLog(ctx, tailArgs, pageViews)
	return nil
}

func handleShutdownSignal(sigChan chan os.Signal, pageViews chan PageView) {
	<-sigChan
	log.Println("Shutdown signal received, stopping...")
	close(pageViews)
}

func runPeriodicCleanup(ctx context.Context, db *sql.DB, ticker *time.Ticker, shutdown <-chan os.Signal) {
	performAllCleanups(ctx, db)

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			performAllCleanups(ctx, db)
		case <-shutdown:
			log.Println("Cleanup goroutine shutting down...")
			return
		}
	}
}
