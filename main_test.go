package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRun(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer closeDB(db)

	logPath := filepath.Join(tempDir, "access.log")

	testLogLines := []string{
		`192.168.1.1 - - [24/Dec/2025:10:30:45 +0000] "GET /index.html HTTP/1.1" 200 1234 "https://google.com" "Mozilla/5.0"`,
		`10.0.0.5 - - [24/Dec/2025:10:31:00 +0000] "GET /api/data HTTP/1.1" 200 5678 "-" "curl/7.68.0"`,
		`192.168.1.100 - - [24/Dec/2025:10:31:15 +0000] "GET /style.css HTTP/1.1" 200 900 "https://example.com" "Mozilla/5.0"`,
	}
	createTestLogFile(t, logPath, testLogLines)

	pageViews := make(chan PageView, 100)
	var wg sync.WaitGroup
	wg.Add(1)
	go dbWriterWithWaitGroup(db, pageViews, &wg)

	tailArgs := []string{"-n", "+1", logPath}
	tailLog(tailArgs, pageViews)
	close(pageViews)
	wg.Wait()

	query := `SELECT * FROM pageviews`

	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("could not query the database: %v", err)
	}
	defer rows.Close()

	var pageViewEntries []PageView
	for rows.Next() {
		var id string
		var pageViewEntry PageView
		err := rows.Scan(
			&id,
			&pageViewEntry.Timestamp,
			&pageViewEntry.Path,
			&pageViewEntry.Referrer,
			&pageViewEntry.UserAgent,
			&pageViewEntry.StatusCode,
			&pageViewEntry.BytesSent,
			&pageViewEntry.IPHash,
			&pageViewEntry.IsBot,
			&pageViewEntry.IsStatic,
		)
		if err != nil {
			log.Fatalf("unable to parse database pageview output, %v", err)
		}
		pageViewEntries = append(pageViewEntries, pageViewEntry)
	}

	if len(pageViewEntries) != 3 {
		t.Fatalf("Expected 3 rows, got %d", len(pageViewEntries))
	}

	if !pageViewEntries[1].IsBot {
		t.Errorf("Expected curl user agent to be detected as bot")
	}

	if !pageViewEntries[2].IsStatic {
		t.Errorf("Expected .css file to be detected as static asset")
	}

	if pageViewEntries[0].IsBot {
		t.Errorf("Expected Mozilla user agent to not be detected as bot")
	}

	if pageViewEntries[0].IsStatic {
		t.Errorf("Expected /index.html to not be detected as static asset")
	}
}

func newTestDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create test db directory: %w", err)
	}
	return openDB(dbPath)
}

func setupTestDB(t *testing.T) (*sql.DB, string) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := newTestDB(dbPath)
	if err != nil {
		t.Fatalf("Unable to create test database: %v", err)
	}
	return db, tempDir
}

func dbWriterWithWaitGroup(db *sql.DB, pageViews <-chan PageView, wg *sync.WaitGroup) {
	defer wg.Done()
	dbWriter(db, pageViews)
}

func createTestLogFile(t *testing.T, logPath string, logLines []string) {
	dir := filepath.Dir(logPath)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	file, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed ot create log file: %v", err)
	}
	defer file.Close()

	for _, line := range logLines {
		_, err := file.WriteString(line + "\n")
		if err != nil {
			t.Fatalf("Failed to write log line: %v", err)
		}
	}
}
