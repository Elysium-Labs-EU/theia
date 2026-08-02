package database_test

import (
	"database/sql"
	"embed"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Elysium-Labs-EU/theia/database"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
)

//go:embed migrations/*.sql
var testMigrationsFS embed.FS

func TestMigrations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := database.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	t.Log("Running up migrations...")
	if migrationsErr := database.RunMigrations(db, testMigrationsFS, "migrations"); migrationsErr != nil {
		t.Fatalf("Failed to run up migrations: %v", migrationsErr)
	}

	expectedVersion := getExpectedVersion(t, "migrations")
	version, dirty, err := database.GetCurrentVersion(db, testMigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}
	if dirty {
		t.Fatalf("Database is dirty after up migration - this should never happen in a successful migration")
	}
	if version != expectedVersion {
		t.Fatalf("Expected version %d after up migration, got version %d", expectedVersion, version)
	}
	t.Logf("Up migration successful, database at version %d", version)

	t.Log("Verifying tables exist...")
	verifyTablesExist(t, db)
	t.Log("All expected tables exist with correct structure")

	t.Log("Testing data insertion...")
	testDataInsertion(t, db)
	t.Log("Data insertion successful")

	t.Log("Running down migration...")
	if migrationsErr := runDownMigration(db, "migrations"); migrationsErr != nil {
		t.Fatalf("Failed to run down migration: %v", migrationsErr)
	}
	t.Log("Down migration successful")

	t.Log("Verifying tables are removed...")
	verifyTablesRemoved(t, db)
	t.Log("All tables successfully removed")

	t.Log("Running up migration again to test reversibility...")
	if migrationsErr := database.RunMigrations(db, testMigrationsFS, "migrations"); migrationsErr != nil {
		t.Fatalf("Failed to run up migrations second time: %v", migrationsErr)
	}
	t.Log("Second up migration successful - migrations are fully reversible")

	expectedVersion = getExpectedVersion(t, "migrations")
	version, dirty, err = database.GetCurrentVersion(db, testMigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("Failed to get final version: %v", err)
	}
	if dirty {
		t.Fatalf("Database is dirty after second up migration")
	}
	if version != expectedVersion {
		t.Fatalf("Expected version %d after second up migration, got version %d", expectedVersion, version)
	}
	t.Log("All migration tests passed!")
}

// TestConcurrentMigrationsSameDBPath reproduces the migration half of issue #23:
// several theia processes starting against the same db-path each run migrations
// at once. Without the cross-process migration lock, golang-migrate's dirty-state
// bookkeeping races and one migrator aborts with "Dirty database version". With
// AcquireMigrationLock serializing them, every migrator succeeds and the schema
// ends up clean at the expected version.
func TestConcurrentMigrationsSameDBPath(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "race.db")

	const migrators = 8

	var wg sync.WaitGroup
	errs := make([]error, migrators)

	for i := range migrators {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = migrateWithLock(t, dbPath)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("migrator %d failed racing on the same db-path: %v", i, err)
		}
	}

	verifyMigratedVersion(t, dbPath, "migrations")
}

// migrateWithLock opens db, acquires the cross-process migration lock, and runs
// the up migrations — the per-goroutine unit of work raced by
// TestConcurrentMigrationsSameDBPath.
func migrateWithLock(t *testing.T, dbPath string) error {
	t.Helper()

	db, err := database.Open(t.Context(), dbPath)
	if err != nil {
		return err
	}
	defer database.Close(db) //nolint:errcheck // cleanup only

	release, err := database.AcquireMigrationLock(dbPath)
	if err != nil {
		return err
	}
	defer release() //nolint:errcheck // cleanup only

	return database.RunMigrations(db, testMigrationsFS, "migrations")
}

func verifyMigratedVersion(t *testing.T, dbPath, migrationsDir string) {
	t.Helper()

	db, err := database.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close(db) //nolint:errcheck // cleanup only

	expectedVersion := getExpectedVersion(t, migrationsDir)
	version, dirty, err := database.GetCurrentVersion(db, testMigrationsFS, migrationsDir)
	if err != nil {
		t.Fatalf("failed to get current version: %v", err)
	}
	if dirty {
		t.Fatalf("database left dirty after concurrent migrations")
	}
	if version != expectedVersion {
		t.Fatalf("expected version %d after concurrent migrations, got %d", expectedVersion, version)
	}
}

func getExpectedVersion(t *testing.T, migrationsDir string) uint {
	t.Helper()

	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("Failed to read migrations directory: %v", err)
	}

	var migrationCount uint
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".up.sql") {
			migrationCount++
		}
	}

	if migrationCount == 0 {
		t.Fatalf("No migrations found in directory - test cannot proceed")
	}

	return migrationCount
}

func verifyTablesExist(t *testing.T, db *sql.DB) {
	t.Helper()

	expectedTables := []string{
		"visitor_days",
		"hourly_stats",
		"hourly_status_codes",
		"hourly_referrers",
	}

	for _, tableName := range expectedTables {
		var count int
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
		err := db.QueryRow(query, tableName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to check for table %s: %v", tableName, err)
		}
		if count != 1 {
			t.Errorf("Expected table %s to exist, but it doesn't", tableName)
		}
	}
}

func verifyTablesRemoved(t *testing.T, db *sql.DB) {
	t.Helper()

	expectedTables := []string{
		"visitor_days",
		"hourly_stats",
		"hourly_status_codes",
		"hourly_referrers",
	}

	for _, tableName := range expectedTables {
		var count int
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
		err := db.QueryRow(query, tableName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to check for table %s: %v", tableName, err)
		}
		if count != 0 {
			t.Errorf("Expected table %s to be removed, but it still exists", tableName)
		}
	}
}

func testDataInsertion(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO visitor_days (hash, host, year, year_day, first_seen)
		VALUES (?, ?, ?, ?, datetime('now'))
	`, "test_hash_123", "example.com", 2024, 1)
	if err != nil {
		t.Fatalf("Failed to insert into visitor_days: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO hourly_stats (hour, year_day, year, path, host, page_views, is_static, bot_views)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, 14, 1, 2024, "/test", "example.com", 1, 0, 0)
	if err != nil {
		t.Fatalf("Failed to insert into hourly_stats: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO hourly_status_codes (hour, year_day, year, path, host, status_code, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, 14, 1, 2024, "/test", "example.com", 200, 1)
	if err != nil {
		t.Fatalf("Failed to insert into hourly_status_codes: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO hourly_referrers (hour, year_day, year, path, host, referrer, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, 14, 1, 2024, "/test", "example.com", "https://google.com", 1)
	if err != nil {
		t.Fatalf("Failed to insert into hourly_referrers: %v", err)
	}

	var hash string
	err = db.QueryRow(`SELECT hash FROM visitor_days WHERE hash = ?`, "test_hash_123").Scan(&hash)
	if err != nil {
		t.Fatalf("Failed to read back inserted data: %v", err)
	}
	if hash != "test_hash_123" {
		t.Errorf("Expected hash 'test_hash_123', got '%s'", hash)
	}
}

func runDownMigration(db *sql.DB, migrationsPath string) error {
	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return err
	}

	// Now create the migrate instance with our driver
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"sqlite",
		driver,
	)
	if err != nil {
		return err
	}

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	return nil
}
