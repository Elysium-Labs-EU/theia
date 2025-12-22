package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"time"

	_ "modernc.org/sqlite"
)

type PageView struct {
	Timestamp time.Time
	Path      string
	Referrer  string
	UserAgent string
	IPHash    string
}

func main() {
	db, err := openDB("./pageviews.db")
	if err != nil {
		log.Fatal(err)
	}
	// TODO: What to do with this error return?
	defer closeDB(db)

	pageViews := make(chan PageView, 100)

	go dbWriter(db, pageViews)

	tailLog("/var/log/nginx/access.log", pageViews)
}

func openDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("could not open database: %w", err)
	}

	err = initTable(db)

	if err != nil {
		return nil, fmt.Errorf("could not initialize table: %w", err)
	}

	return db, nil
}

func closeDB(db *sql.DB) error {
	return db.Close()
}

func initTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS pageviews (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME,
		path TEXT,
		referrer TEXT,
		user_agent TEXT,
		ip_hash TEXT
	)`

	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("could not create pageviews table, %w", err)
	}
	return nil
}

func dbWriter(db *sql.DB, pageViews <-chan PageView) {
	for {
		pageView, ok := <-pageViews

		if !ok {
			break
		}

		pageViewQuery := `
		INSERT INTO pageviews (timestamp, path, referrer, user_agent, ip_hash)
		VALUES (?, ?, ?, ?, ?)
		`

		_, err := db.Exec(pageViewQuery, pageView.Timestamp, pageView.Path, pageView.Referrer, pageView.UserAgent, pageView.IPHash)
		if err != nil {
			fmt.Printf("Unable to write pageview into database, got: %v", err)
		}
	}
}

func tailLog(logPath string, pageViews chan<- PageView) {
	tailArgs := []string{"-f", logPath}
	tailLogCommand := exec.Command("tail", tailArgs...)
	readCloser, err := tailLogCommand.StdoutPipe()
	if err != nil {
		fmt.Printf("error occured during setting up log reading, got: %v", err)
		return
	}

	err = tailLogCommand.Start()
	if err != nil {
		fmt.Printf("error occured during setting starting log reading, got: %v", err)
		return
	}

	scanner := bufio.NewScanner(readCloser)
	for scanner.Scan() {
		line := scanner.Text()
		pageView, err := parseNginxLog(line)
		if err != nil {
			fmt.Printf("error occured during parsing of the line, got: %v", err)
			continue
		}
		pageViews <- pageView
	}
}

func parseNginxLog(line string) (PageView, error) {
	pattern := `^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" \d+ \d+ "([^"]*)" "([^"]*)"`

	regex := regexp.MustCompile(pattern)
	matches := regex.FindStringSubmatch(line)
	if matches == nil {
		return PageView{}, fmt.Errorf("failed to parse log line")
	}
	ip := matches[1]
	timestamp := matches[2]
	path := matches[4]
	referrer := matches[5]
	user_agent := matches[6]

	hashedIPAddress := sha256.Sum256([]byte(ip))
	hexedIPAddress := hex.EncodeToString(hashedIPAddress[:])

	parsedTimestamp, err := time.Parse("02/Jan/2006:15:04:05 -0700", timestamp)
	if err != nil {
		return PageView{}, fmt.Errorf("failed to parse timestamp")
	}

	return PageView{
		Timestamp: parsedTimestamp,
		Path:      path,
		Referrer:  referrer,
		UserAgent: user_agent,
		IPHash:    hexedIPAddress,
	}, nil
}
