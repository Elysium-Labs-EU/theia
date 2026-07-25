package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// busyRetryTimeout bounds how long Open waits for a racing opener to release
// the write lock it takes while switching the database to WAL journal mode.
// Matches the busy_timeout DSN pragma so runtime and startup contention wait
// the same 5s before giving up. See issue #23.
const busyRetryTimeout = 5 * time.Second

func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	if err := checkParentDir(dbPath); err != nil {
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	// WAL lets readers and writers work concurrently instead of blocking on
	// SQLite's default rollback-journal exclusive lock; busy_timeout makes a
	// writer that still loses that race retry for 5s instead of failing
	// immediately with SQLITE_BUSY, which is what daemon's startup cleanup
	// queries were hitting against a database opened without either pragma.
	//
	// Pragma order matters: the driver applies _pragma DSN params in sequence
	// on every new connection, and switching to WAL itself takes a brief write
	// lock. busy_timeout MUST come first so that switch already retries for 5s;
	// with journal_mode first, two daemons opening the same db-path at once race
	// on the WAL switch and the loser gets an immediate, unhandled SQLITE_BUSY
	// (the very crash busy_timeout was meant to prevent) before the timeout is
	// even armed. See issue #23.
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}

	if err := pingWithBusyRetry(ctx, db, busyRetryTimeout); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("could not close failing connection to database: %w", err)
		}
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	return db, nil
}

// pingWithBusyRetry pings the database, retrying with backoff while the error is
// SQLITE_BUSY. The busy_timeout pragma alone does not cover the exclusive lock
// SQLite takes to switch a fresh database into WAL mode: two daemons opening the
// same db-path at once can each hold a shared lock while waiting to upgrade to
// exclusive, and SQLite returns that conflict as an immediate SQLITE_BUSY rather
// than invoking the busy handler. A failed connection attempt releases its lock,
// so retrying lets the losers converge once the winner finishes the conversion.
func pingWithBusyRetry(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	backoff := 10 * time.Millisecond

	for {
		err := db.PingContext(ctx)
		if err == nil || !isBusyErr(err) || time.Now().After(deadline) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		if backoff < 200*time.Millisecond {
			backoff *= 2
		}
	}
}

// isBusyErr reports whether err is a SQLite lock-contention error. The modernc
// driver surfaces this as a message rather than a typed sentinel, so match the
// text it emits for SQLITE_BUSY.
func isBusyErr(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked")
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
