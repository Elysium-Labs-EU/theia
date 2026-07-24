package database_test

import (
	"os"
	"path/filepath"
	"strings"
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
