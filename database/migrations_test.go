package database_test

import (
	"database/sql"
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"theia/database"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
)

//go:embed migrations/*.sql
var testMigrationsFS embed.FS

func TestMigrations(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer database.Close(db)

	t.Log("Running up migrations...")
	if err := database.RunMigrations(db, testMigrationsFS, "migrations"); err != nil {
		t.Fatalf("Failed to run up migrations: %v", err)
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
	if err := runDownMigration(db, "migrations"); err != nil {
		t.Fatalf("Failed to run down migration: %v", err)
	}
	t.Log("Down migration successful")

	t.Log("Verifying tables are removed...")
	verifyTablesRemoved(t, db)
	t.Log("All tables successfully removed")

	t.Log("Running up migration again to test reversibility...")
	if err := database.RunMigrations(db, testMigrationsFS, "migrations"); err != nil {
		t.Fatalf("Failed to run up migrations second time: %v", err)
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

func getExpectedVersion(t *testing.T, migrationsDir string) uint {
	t.Helper()

	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("Failed to read migrations directory: %v", err)
	}

	var migrationCount uint
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".up.sql") {
			migrationCount += 1
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
		"visitor_hashes",
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
		"visitor_hashes",
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
		INSERT INTO visitor_hashes (hash, hour_bucket, first_seen)
		VALUES (?, ?, datetime('now'))
	`, "test_hash_123", 14)
	if err != nil {
		t.Fatalf("Failed to insert into visitor_hashes: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO hourly_stats (hour, year_day, year, path, host, page_views, is_static, unique_visitors, bot_views)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 14, 1, 2024, "/test", "example.com", 1, 0, 1, 0)
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
	err = db.QueryRow(`SELECT hash FROM visitor_hashes WHERE hash = ?`, "test_hash_123").Scan(&hash)
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

	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}
