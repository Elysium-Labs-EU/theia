package database_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Elysium-Labs-EU/theia/database"
)

func TestOpenMissingParentDir(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "nonexistent-dir-xyz", "foo.db")

	db, err := database.Open(t.Context(), dbPath)
	if err == nil {
		database.Close(db) //nolint:errcheck // cleanup only, test already failed
		t.Fatalf("expected error opening database with missing parent dir, got nil")
	}
	if db != nil {
		t.Errorf("expected nil *sql.DB on error, got non-nil")
	}

	msg := err.Error()
	if strings.Contains(msg, "out of memory") {
		t.Errorf("error message should not mention memory for a missing directory, got: %v", err)
	}
	if !strings.Contains(msg, "does not exist") {
		t.Errorf("expected error to say the parent directory does not exist, got: %v", err)
	}
}

func TestOpenParentIsFile(t *testing.T) {
	tempDir := t.TempDir()
	notADir := filepath.Join(tempDir, "not-a-dir")
	if err := os.WriteFile(notADir, []byte(""), 0o600); err != nil {
		t.Fatalf("failed to set up test file: %v", err)
	}
	dbPath := filepath.Join(notADir, "foo.db")

	db, err := database.Open(t.Context(), dbPath)
	if err == nil {
		database.Close(db) //nolint:errcheck // cleanup only, test already failed
		t.Fatalf("expected error opening database whose parent is a file, got nil")
	}

	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("expected error to say the parent path is not a directory, got: %v", err)
	}
}

// TestOpenConcurrentSameDBPath reproduces issue #23: two daemons racing to open
// the same db-path. Opening triggers a switch to WAL journal mode, which takes a
// brief write lock; the busy_timeout pragma must be armed before that switch so
// the loser retries instead of crashing with an unhandled SQLITE_BUSY. With the
// pragmas in the wrong DSN order this fails; with busy_timeout first it passes.
func TestOpenConcurrentSameDBPath(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "race.db")

	const openers = 8

	var wg sync.WaitGroup
	errs := make([]error, openers)

	for i := range openers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			db, err := database.Open(t.Context(), dbPath)
			if err != nil {
				errs[idx] = err
				return
			}
			database.Close(db) //nolint:errcheck // cleanup only
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			continue
		}
		msg := err.Error()
		if strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked") {
			t.Errorf("opener %d hit an unhandled busy error racing on the same db-path: %v", i, err)
			continue
		}
		t.Errorf("opener %d failed unexpectedly: %v", i, err)
	}
}
