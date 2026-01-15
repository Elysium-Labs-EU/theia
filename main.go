package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"theia/database"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed database/migrations/*.sql
var migrationsFS embed.FS

type PageView struct {
	Timestamp  time.Time
	Host       string
	Path       string
	Referrer   string
	UserAgent  string
	StatusCode int
	BytesSent  int
	IDHash     string
	IsBot      bool
	IsStatic   bool
}

type VisitorHash struct {
	Hash       string
	HourBucket int
	FirstSeen  time.Time
}

type HourlyStatusCodes struct {
	Hour       int
	YearDay    int
	Year       int
	Path       string
	Host       string
	StatusCode int
	Count      int
}

type HourlyReferrers struct {
	Hour     int
	YearDay  int
	Year     int
	Path     string
	Host     string
	Referrer string
	Count    int
}

type HourlyStats struct {
	Hour           int
	YearDay        int
	Year           int
	Path           string
	Host           string
	Pageviews      int
	IsStatic       bool
	UniqueVisitors int
	BotViews       int
}

func main() {
	if err := run("./theia.db", "/var/log/nginx/access.log"); err != nil {
		log.Fatal(err)
	}
}

func run(dbPath string, logPath string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close(db)

	if err := database.RunMigrations(db, migrationsFS, "database/migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	version, dirty, err := database.GetCurrentVersion(db, migrationsFS, "database/migrations")
	if err != nil {
		log.Printf("Warning: Could not get schema version: %v", err)
	} else {
		log.Printf("Database schema version: %d (dirty: %v)", version, dirty)
		if dirty {
			log.Fatal("Database is in a dirty state. Manual intervention required.")
		}
	}

	pageViews := make(chan PageView, 100)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go processPageviews(db, pageViews)
	go runPeriodicCleanup(db, time.NewTicker(12*time.Hour), sigChan)

	go handleShutdownSignal(sigChan, pageViews)

	tailArgs := []string{"-f", logPath}
	tailLog(tailArgs, pageViews)
	return nil
}

func processPageviews(db *sql.DB, pageViews <-chan PageView) {
	for {
		pageView, ok := <-pageViews

		if !ok {
			break
		}

		visitorHashesQuery := `
		SELECT hash
		FROM visitor_hashes
		WHERE hash = ?`

		row := db.QueryRow(visitorHashesQuery, pageView.IDHash)

		var visitorHash string
		err := row.Scan(&visitorHash)
		if err == sql.ErrNoRows {
			visitorHashesUpdateQuery := `
			INSERT INTO visitor_hashes (hash, hour_bucket, first_seen)
			VALUES (?, ?, ?)
			`

			_, err = db.Exec(visitorHashesUpdateQuery, pageView.IDHash, pageView.Timestamp.Hour(), pageView.Timestamp.Format("2006-01-02 15:04:05"))
			if err != nil {
				fmt.Printf("Unable to write visitor hash into database, got: %v\n", err)
			}
		} else if err != nil {
			fmt.Printf("Unable to parse visitor hash from database, got: %v\n", err)
		}

		visitorHourBucketQuery := `
		SELECT COUNT(hash)
		FROM visitor_hashes
		WHERE hour_bucket = ?`

		visitorHashesCount := db.QueryRow(visitorHourBucketQuery, pageView.Timestamp.Hour())
		var visitorHashesCountResult int

		err = visitorHashesCount.Scan(&visitorHashesCountResult)
		if err == sql.ErrNoRows {
			fmt.Print("No visitors found, should have at least one visitor at this point")
		} else if err != nil {
			fmt.Printf("Unable to retrieve count of visitor hash from database, got: %v\n", err)
		}

		hourlyStatsUpdateQuery := `
		INSERT INTO hourly_stats (hour, year_day, year, path, host, page_views, is_static, unique_visitors, bot_views)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host) DO UPDATE SET
			page_views = page_views + ?,
			unique_visitors = ?,
			bot_views = bot_views + ? 
		`

		pageViewIncrement := 0
		botViewIncrement := 0
		if pageView.IsBot {
			botViewIncrement = 1
		} else {
			pageViewIncrement = 1
		}

		_, err = db.Exec(hourlyStatsUpdateQuery,
			pageView.Timestamp.Hour(),
			pageView.Timestamp.YearDay(),
			pageView.Timestamp.Year(),
			pageView.Path,
			pageView.Host,
			pageViewIncrement,
			pageView.IsStatic,
			visitorHashesCountResult,
			botViewIncrement,
			pageViewIncrement,
			visitorHashesCountResult,
			botViewIncrement)
		if err != nil {
			fmt.Printf("Unable to write hourly stats into database, got: %v\n", err)
		}

		hourlyStatusCodesUpdateQuery := `
		INSERT INTO hourly_status_codes (hour, year_day, year, path, host, status_code, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host, status_code) DO UPDATE SET
			count = count + ?
		`
		_, err = db.Exec(hourlyStatusCodesUpdateQuery,
			pageView.Timestamp.Hour(),
			pageView.Timestamp.YearDay(),
			pageView.Timestamp.Year(),
			pageView.Path,
			pageView.Host,
			pageView.StatusCode,
			1,
			1)
		if err != nil {
			fmt.Printf("Unable to write hourly status codes into database, got: %v\n", err)
		}

		hourlyReferrersUpdateQuery := `
		INSERT INTO hourly_referrers (hour, year_day, year, path, host, referrer, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(hour, year_day, year, path, host, referrer) DO UPDATE SET
			count = count + ?
		`

		_, err = db.Exec(hourlyReferrersUpdateQuery,
			pageView.Timestamp.Hour(),
			pageView.Timestamp.YearDay(),
			pageView.Timestamp.Year(),
			pageView.Path,
			pageView.Host,
			pageView.Referrer,
			1,
			1)
		if err != nil {
			fmt.Printf("Unable to write hourly referrers into database, got: %v\n", err)
		}
	}
}

func tailLog(tailArgs []string, pageViews chan<- PageView) {
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

func getDefaultHost() string {
	if host := os.Getenv("THEIA_DEFAULT_HOST"); host != "" {
		return host
	}
	return "default"
}

var (
	regexWithHost = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) (\d+) "([^"]*)" "([^"]*)" "([^"]*)"`)
	regexStandard = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) (\d+) "([^"]*)" "([^"]*)"`)
)

func determineMatchingPattern(line string) (matches []string, withHost bool, err error) {
	matchesWithHost := regexWithHost.FindStringSubmatch(line)

	if matchesWithHost != nil {
		return matchesWithHost, true, nil
	}

	matchesStandard := regexStandard.FindStringSubmatch(line)
	if matchesStandard != nil {
		return matchesStandard, false, nil
	}

	return nil, false, fmt.Errorf("failed to parse log line")
}

func parseNginxLog(line string) (PageView, error) {
	matches, withHost, err := determineMatchingPattern(line)
	if err != nil {
		return PageView{}, err
	}

	ip := matches[1]
	timestamp := matches[2]
	// httpMethod := matches[3]
	path := matches[4]
	statusCode := matches[5]
	bytesSent := matches[6]
	referrer := matches[7]
	userAgent := matches[8]
	var host string
	if withHost {
		host = matches[9]
	} else {
		host = getDefaultHost()
	}

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

	hashInput := ip + userAgent + time.Now().Format("2006-01-02")
	hashedID := sha256.Sum256([]byte(hashInput))
	hashedIDString := hex.EncodeToString(hashedID[:])

	isBot := detectBot(userAgent)

	isStatic := isStaticAsset(path)

	return PageView{
		Timestamp:  parsedTimestamp,
		Host:       host,
		Path:       path,
		StatusCode: statusCodeAsInt,
		BytesSent:  bytesSentAsInt,
		Referrer:   referrer,
		UserAgent:  userAgent,
		IDHash:     hashedIDString,
		IsBot:      isBot,
		IsStatic:   isStatic,
	}, nil
}

func handleShutdownSignal(sigChan chan os.Signal, pageViews chan PageView) {
	<-sigChan
	log.Println("Shutdown signal received, stopping...")
	close(pageViews)
}

func runPeriodicCleanup(db *sql.DB, ticker *time.Ticker, shutdown <-chan os.Signal) {
	performAllCleanups(db)

	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			performAllCleanups(db)
		case <-shutdown:
			log.Println("Cleanup goroutine shutting down...")
			return
		}
	}
}

func performAllCleanups(db *sql.DB) {
	if deleted, err := dbCleanUpOldHourlyStats(db); err != nil {
		log.Printf("Hourly stats cleanup error: %v", err)
	} else {
		log.Printf("Cleaned up %d old hourly stats records", deleted)
	}

	if deleted, err := dbCleanUpOldHourlyStatusCodes(db); err != nil {
		log.Printf("Status codes cleanup error: %v", err)
	} else {
		log.Printf("Cleaned up %d old status code records", deleted)
	}

	if deleted, err := dbCleanUpOldHourlyReferrer(db); err != nil {
		log.Printf("Referrers cleanup error: %v", err)
	} else {
		log.Printf("Cleaned up %d old referrer records", deleted)
	}

	if deleted, err := dbCleanUpOldVisitorHashes(db); err != nil {
		log.Printf("Visitor hashes cleanup error: %v", err)
	} else {
		log.Printf("Cleaned up %d old visitor hash records", deleted)
	}
}

func dbCleanUpOldHourlyStats(db *sql.DB) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -60)
	cutoffYear := cutoffDate.Year()
	cutoffYearDay := cutoffDate.YearDay()

	query := `
	DELETE FROM hourly_stats 
	WHERE year < ? 
	   OR (year = ? AND year_day < ?)`

	result, err := db.Exec(query, cutoffYear, cutoffYear, cutoffYearDay)
	if err != nil {
		return 0, fmt.Errorf("could not delete old hourly stats records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	return rowsDeleted, nil
}

func dbCleanUpOldHourlyStatusCodes(db *sql.DB) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -60)
	cutoffYear := cutoffDate.Year()
	cutoffYearDay := cutoffDate.YearDay()

	query := `
	DELETE FROM hourly_status_codes 
	WHERE year < ? 
	   OR (year = ? AND year_day < ?)`

	result, err := db.Exec(query, cutoffYear, cutoffYear, cutoffYearDay)
	if err != nil {
		return 0, fmt.Errorf("could not delete old hourly status codes records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	return rowsDeleted, nil
}

func dbCleanUpOldHourlyReferrer(db *sql.DB) (int64, error) {
	cutoffDate := time.Now().AddDate(0, 0, -60)
	cutoffYear := cutoffDate.Year()
	cutoffYearDay := cutoffDate.YearDay()

	query := `
	DELETE FROM hourly_referrers 
	WHERE year < ? 
	   OR (year = ? AND year_day < ?)`

	result, err := db.Exec(query, cutoffYear, cutoffYear, cutoffYearDay)
	if err != nil {
		return 0, fmt.Errorf("could not delete old hourly referrer records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	return rowsDeleted, nil
}

func dbCleanUpOldVisitorHashes(db *sql.DB) (int64, error) {
	query := `
	DELETE FROM visitor_hashes
	WHERE datetime(first_seen) < datetime('now', '-1 day');`

	result, err := db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("could not delete old records, %w", err)
	}

	rowsDeleted, _ := result.RowsAffected()
	return rowsDeleted, nil
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
