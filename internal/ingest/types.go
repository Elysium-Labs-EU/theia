package ingest

import "time"

type PageView struct {
	Timestamp  time.Time
	Host       string
	Path       string
	Referrer   string
	UserAgent  string
	IDHash     string
	StatusCode int
	BytesSent  int
	IsBot      bool
	IsStatic   bool
}

type VisitorDay struct {
	FirstSeen time.Time
	Hash      string
	Host      string
	Year      int
	YearDay   int
}

type HourlyStatusCodes struct {
	Path       string
	Host       string
	Hour       int
	YearDay    int
	Year       int
	StatusCode int
	Count      int
}

type HourlyReferrers struct {
	Path     string
	Host     string
	Referrer string
	Hour     int
	YearDay  int
	Year     int
	Count    int
}

type HourlyStats struct {
	Path      string
	Host      string
	Hour      int
	YearDay   int
	Year      int
	Pageviews int
	BotViews  int
	IsStatic  bool
}
