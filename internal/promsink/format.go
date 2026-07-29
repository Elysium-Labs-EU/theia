// Package promsink renders theia's pageview, status-code, and referrer
// counts as Prometheus text-exposition metrics, served on their own
// address independent of the bearer-authed JSON/CSV API in apiserver.
package promsink

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Elysium-Labs-EU/theia/internal/query"
)

// Snapshot is the narrow slice of query results the metrics endpoint
// renders — exactly what a scrape needs, nothing else.
type Snapshot struct {
	Paths       []query.PathStat
	StatusCodes []query.StatusStat
	Referrers   []query.ReferrerStat
}

// Render formats s as Prometheus text-exposition format (version 0.0.4):
// https://github.com/prometheus/docs/blob/main/content/docs/instrumenting/exposition_formats.md
func Render(s Snapshot) string {
	var b strings.Builder

	b.WriteString("# HELP theia_pageviews_total Total page views by host and path.\n")
	b.WriteString("# TYPE theia_pageviews_total counter\n")
	for _, p := range s.Paths {
		fmt.Fprintf(&b, "theia_pageviews_total{host=%s,path=%s} %d\n", quote(p.Host), quote(p.Path), p.Pageviews)
	}

	b.WriteString("# HELP theia_status_codes_total Total responses by HTTP status code.\n")
	b.WriteString("# TYPE theia_status_codes_total counter\n")
	for _, sc := range s.StatusCodes {
		fmt.Fprintf(&b, "theia_status_codes_total{status_code=%s} %d\n", quote(strconv.Itoa(sc.StatusCode)), sc.Count)
	}

	b.WriteString("# HELP theia_referrers_total Total page views by referrer.\n")
	b.WriteString("# TYPE theia_referrers_total counter\n")
	for _, r := range s.Referrers {
		fmt.Fprintf(&b, "theia_referrers_total{referrer=%s} %d\n", quote(r.Referrer), r.Count)
	}

	return b.String()
}

// quote renders v as a double-quoted Prometheus label value, escaping
// backslashes, quotes, and newlines per the text exposition format spec —
// theia's own path/referrer values are attacker-influenced (raw request
// paths and Referer headers from an access log), so an unescaped one could
// otherwise break out of the label and corrupt the exposition stream.
func quote(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return `"` + v + `"`
}
