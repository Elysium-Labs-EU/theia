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
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type PageView struct {
	Timestamp  time.Time
	Path       string
	Referrer   string
	UserAgent  string
	StatusCode int
	BytesSent  int
	IPHash     string
	IsBot      bool
	IsStatic   bool
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
	go cleanupOldRecords(db)

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
		status_code INTEGER,
		bytes_sent INTEGER,
		ip_hash TEXT,
		is_bot BOOLEAN,
		is_static BOOLEAN
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
		INSERT INTO pageviews (timestamp, path, referrer, user_agent, status_code, bytes_sent, ip_hash, is_bot, is_static)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		_, err := db.Exec(pageViewQuery, pageView.Timestamp, pageView.Path, pageView.Referrer, pageView.UserAgent, pageView.StatusCode, pageView.BytesSent, pageView.IPHash, pageView.IsBot, pageView.IsStatic)
		if err != nil {
			fmt.Printf("Unable to write pageview into database, got: %v\n", err)
		}
	}
}

func tailLog(logPath string, pageViews chan<- PageView) {
	tailArgs := []string{"-f", logPath}
	tailLogCommand := exec.Command("tail", tailArgs...)
	readCloser, err := tailLogCommand.StdoutPipe()
	if err != nil {
		fmt.Printf("error occured during setting up log reading, got: %v\n", err)
		return
	}

	err = tailLogCommand.Start()
	if err != nil {
		fmt.Printf("error occured during starting log reading, got: %v\n", err)
		return
	}

	scanner := bufio.NewScanner(readCloser)
	for scanner.Scan() {
		line := scanner.Text()
		pageView, err := parseNginxLog(line)
		if err != nil {
			fmt.Printf("error occured during parsing of the line, got: %v\n", err)
			continue
		}
		pageViews <- pageView
	}
}

func parseNginxLog(line string) (PageView, error) {
	pattern := `^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) (\d+) "([^"]*)" "([^"]*)"`

	regex := regexp.MustCompile(pattern)
	matches := regex.FindStringSubmatch(line)
	if matches == nil {
		return PageView{}, fmt.Errorf("failed to parse log line")
	}
	ip := matches[1]
	timestamp := matches[2]
	// httpMethod := matches[3]
	path := matches[4]
	statusCode := matches[5]
	bytesSent := matches[6]
	referrer := matches[7]
	userAgent := matches[8]

	parsedTimestamp, err := time.Parse("02/Jan/2006:15:04:05 -0700", timestamp)
	if err != nil {
		return PageView{}, fmt.Errorf("failed to parse timestamp")
	}
	statusCodeAsInt, err := strconv.Atoi(statusCode)
	if err != nil {
		return PageView{}, fmt.Errorf("failed to parse statuscode")
	}
	bytesSentAsInt, err := strconv.Atoi(bytesSent)
	if err != nil {
		return PageView{}, fmt.Errorf("failed to parse bytes sent")
	}

	hashedIPAddress := sha256.Sum256([]byte(ip))
	hexedIPAddress := hex.EncodeToString(hashedIPAddress[:])

	isBot := detectBot(userAgent)

	isStatic := isStaticAsset(path)

	return PageView{
		Timestamp:  parsedTimestamp,
		Path:       path,
		StatusCode: statusCodeAsInt,
		BytesSent:  bytesSentAsInt,
		Referrer:   referrer,
		UserAgent:  userAgent,
		IPHash:     hexedIPAddress,
		IsBot:      isBot,
		IsStatic:   isStatic,
	}, nil
}

func cleanupOldRecords(db *sql.DB) {
	err := dbCleanUpOldLogs(db)
	if err != nil {
		fmt.Printf("Initial cleanup error: %v\n", err)
	}

	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		err := dbCleanUpOldLogs(db)
		if err != nil {
			fmt.Printf("Cleanup error: %v\n", err)
		}
	}
}

func dbCleanUpOldLogs(db *sql.DB) error {
	query := `
	DELETE FROM pageviews WHERE datetime(timestamp) < datetime('now', '-60 days');`

	result, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("could not delete old records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	if rowsDeleted > 0 {
		fmt.Printf("Cleanup: deleted %d old records\n", rowsDeleted)
	}

	return nil
}

func detectBot(userAgent string) bool {
	userAgentLower := strings.ToLower(userAgent)

	botPatterns := []string{
		"bot", "crawler", "spider", "scraper",
		"googlebot", "bingbot", "yandexbot", "baiduspider",
		"facebookexternalhit", "facebot", "twitterbot",
		"slackbot", "telegrambot", "whatsapp",
		"lighthouse", "gtmetrix", "pingdom",
		"headlesschrome", "phantomjs", "selenium",
		"python-requests", "curl", "wget",
		"http", "java/", "go-http-client",
	}

	for _, pattern := range botPatterns {
		if strings.Contains(userAgentLower, pattern) {
			return true
		}
	}

	return false
}

func isStaticAsset(path string) bool {
	staticExtensions := []string{
		".css", ".js", ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg",
		".ico", ".woff", ".woff2", ".ttf", ".eot", ".otf",
		".mp4", ".webm", ".mp3", ".wav", ".pdf", ".zip",
		".xml", ".txt", ".json", ".map",
	}

	pathLower := strings.ToLower(path)

	for _, ext := range staticExtensions {
		if strings.HasSuffix(pathLower, ext) {
			return true
		}
	}

	staticPaths := []string{
		"/assets/", "/static/", "/public/",
		"/images/", "/img/", "/css/", "/js/",
		"/fonts/", "/media/", "/uploads/",
	}

	for _, staticPath := range staticPaths {
		if strings.Contains(pathLower, staticPath) {
			return true
		}
	}

	return false
}
