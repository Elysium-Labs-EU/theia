package database_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
)

func TestAcquireMigrationLockTimeout_Uncontended(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	release, ok, err := database.AcquireMigrationLockTimeout(dbPath, time.Second)
	if err != nil {
		t.Fatalf("AcquireMigrationLockTimeout: %v", err)
	}
	if !ok {
		t.Fatalf("expected to acquire an uncontended lock")
	}
	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestAcquireMigrationLockTimeout_ContendedGivesUp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	holderRelease, err := database.AcquireMigrationLock(dbPath)
	if err != nil {
		t.Fatalf("AcquireMigrationLock (holder): %v", err)
	}
	defer func() {
		if releaseErr := holderRelease(); releaseErr != nil {
			t.Errorf("holderRelease: %v", releaseErr)
		}
	}()

	start := time.Now()
	release, ok, err := database.AcquireMigrationLockTimeout(dbPath, 100*time.Millisecond)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("AcquireMigrationLockTimeout: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false against an already-held lock")
	}
	if release != nil {
		t.Errorf("expected a nil release func when ok=false")
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("returned before the timeout elapsed: %v", elapsed)
	}
}
