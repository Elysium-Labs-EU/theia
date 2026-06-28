package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	regexWithHost = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) (\d+) "([^"]*)" "([^"]*)" "([^"]*)"`)
	regexStandard = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) \S+" (\d+) (\d+) "([^"]*)" "([^"]*)"`)
)

func getDefaultHost() string {
	if host := os.Getenv("THEIA_DEFAULT_HOST"); host != "" {
		return host
	}
	return "default"
}

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
