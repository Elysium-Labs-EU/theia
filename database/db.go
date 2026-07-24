package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	if err := checkParentDir(dbPath); err != nil {
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	// WAL lets readers and writers work concurrently instead of blocking on
	// SQLite's default rollback-journal exclusive lock; busy_timeout makes a
	// writer that still loses that race retry for 5s instead of failing
	// immediately with SQLITE_BUSY, which is what daemon's startup cleanup
	// queries were hitting against a database opened without either pragma.
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("could not close failing connection to database: %w", err)
		}
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	return db, nil
}

// checkParentDir gives an actionable error before the driver's opaque
// SQLITE_CANTOPEN message reaches the user (modernc.org/sqlite mislabels it
// "out of memory (14)" for a missing directory — see issue #17).
func checkParentDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "." {
		return nil
	}

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return fmt.Errorf("parent directory %q does not exist", dir)
	}
	if err != nil {
		return fmt.Errorf("checking parent directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent path %q is not a directory", dir)
	}

	return nil
}

func Close(db *sql.DB) error {
	if err := db.Close(); err != nil {
		return fmt.Errorf("error closing database: %w", err)
	}
	return nil
}
