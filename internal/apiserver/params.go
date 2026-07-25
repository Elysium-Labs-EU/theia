// Package apiserver exposes theia's stats database over a bearer-authed
// HTTP/JSON (and CSV) read API, mirroring what `theia stats` already
// computes but filterable by arbitrary host/date-range/group-by params.
package apiserver

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const dateLayout = "2006-01-02"

const (
	defaultLookbackDays = 7
	defaultTop          = 10
)

type statsParams struct {
	Host    string
	From    time.Time
	To      time.Time
	GroupBy string
	Format  string
}

type breakdownParams struct {
	Host   string
	From   time.Time
	To     time.Time
	Format string
	Top    int
}

func parseStatsParams(q url.Values) (statsParams, error) {
	from, to, err := parseDateRange(q)
	if err != nil {
		return statsParams{}, err
	}
	format, err := parseFormat(q)
	if err != nil {
		return statsParams{}, err
	}
	groupBy := q.Get("group_by")
	if groupBy == "" {
		groupBy = "day"
	}
	if groupBy != "day" && groupBy != "hour" {
		return statsParams{}, fmt.Errorf("invalid group_by %q: must be \"day\" or \"hour\"", groupBy)
	}

	return statsParams{Host: q.Get("host"), From: from, To: to, GroupBy: groupBy, Format: format}, nil
}

func parseBreakdownParams(q url.Values) (breakdownParams, error) {
	from, to, err := parseDateRange(q)
	if err != nil {
		return breakdownParams{}, err
	}
	format, err := parseFormat(q)
	if err != nil {
		return breakdownParams{}, err
	}
	top, err := parseTop(q)
	if err != nil {
		return breakdownParams{}, err
	}

	return breakdownParams{Host: q.Get("host"), From: from, To: to, Format: format, Top: top}, nil
}

func parseDateRange(q url.Values) (from, to time.Time, err error) {
	to = time.Now()
	if s := q.Get("to"); s != "" {
		t, parseErr := time.Parse(dateLayout, s)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid to date %q: want YYYY-MM-DD", s)
		}
		to = t
	}

	from = to.AddDate(0, 0, -defaultLookbackDays)
	if s := q.Get("from"); s != "" {
		t, parseErr := time.Parse(dateLayout, s)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid from date %q: want YYYY-MM-DD", s)
		}
		from = t
	}

	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("from date %q must not be after to date %q", from.Format(dateLayout), to.Format(dateLayout))
	}

	return from, to, nil
}

func parseFormat(q url.Values) (string, error) {
	format := q.Get("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		return "", fmt.Errorf("invalid format %q: must be \"json\" or \"csv\"", format)
	}
	return format, nil
}

func parseTop(q url.Values) (int, error) {
	s := q.Get("top")
	if s == "" {
		return defaultTop, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid top %q: must be a positive integer", s)
	}
	return n, nil
}
