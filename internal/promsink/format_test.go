package promsink

import (
	"strings"
	"testing"

	"github.com/Elysium-Labs-EU/theia/internal/query"
)

func TestRender_EmptySnapshot(t *testing.T) {
	got := Render(Snapshot{})

	want := "# HELP theia_pageviews_total Total page views by host and path.\n" +
		"# TYPE theia_pageviews_total counter\n" +
		"# HELP theia_status_codes_total Total responses by HTTP status code.\n" +
		"# TYPE theia_status_codes_total counter\n" +
		"# HELP theia_referrers_total Total page views by referrer.\n" +
		"# TYPE theia_referrers_total counter\n"

	if got != want {
		t.Errorf("Render() =\n%q\nwant\n%q", got, want)
	}
}

func TestRender_PopulatedSnapshot(t *testing.T) {
	snap := Snapshot{
		Paths: []query.PathStat{
			{Path: "/", Host: "example.com", Pageviews: 42},
		},
		StatusCodes: []query.StatusStat{
			{StatusCode: 200, Count: 100},
		},
		Referrers: []query.ReferrerStat{
			{Referrer: "https://google.com", Count: 7},
		},
	}

	got := Render(snap)

	wantLines := []string{
		`theia_pageviews_total{host="example.com",path="/"} 42`,
		`theia_status_codes_total{status_code="200"} 100`,
		`theia_referrers_total{referrer="https://google.com"} 7`,
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Errorf("Render() missing line %q, got:\n%s", want, got)
		}
	}
}

func TestRender_MultipleEntriesPerFamily(t *testing.T) {
	snap := Snapshot{
		Paths: []query.PathStat{
			{Path: "/", Host: "a.com", Pageviews: 1},
			{Path: "/about", Host: "a.com", Pageviews: 2},
		},
		StatusCodes: []query.StatusStat{
			{StatusCode: 200, Count: 10},
			{StatusCode: 404, Count: 3},
		},
		Referrers: []query.ReferrerStat{
			{Referrer: "direct", Count: 5},
			{Referrer: "https://bing.com", Count: 1},
		},
	}

	got := Render(snap)

	for _, want := range []string{
		`theia_pageviews_total{host="a.com",path="/"} 1`,
		`theia_pageviews_total{host="a.com",path="/about"} 2`,
		`theia_status_codes_total{status_code="200"} 10`,
		`theia_status_codes_total{status_code="404"} 3`,
		`theia_referrers_total{referrer="direct"} 5`,
		`theia_referrers_total{referrer="https://bing.com"} 1`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Render() missing line %q, got:\n%s", want, got)
		}
	}
}

func TestQuote_EscapesSpecialCharacters(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", `"hello"`},
		{"backslash", `back\slash`, `"back\\slash"`},
		{"double_quote", `say "hi"`, `"say \"hi\""`},
		{"newline", "line1\nline2", `"line1\nline2"`},
		{"combined", "a\"b\\c\nd", `"a\"b\\c\nd"`},
		{"empty", "", `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quote(tt.in); got != tt.want {
				t.Errorf("quote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRender_EscapesUntrustedLabelValues(t *testing.T) {
	snap := Snapshot{
		Paths: []query.PathStat{
			{Path: `/evil"} extra_metric 1` + "\n" + `{"`, Host: "a.com", Pageviews: 1},
		},
	}

	got := Render(snap)

	if strings.Contains(got, "extra_metric") == false {
		t.Fatalf("test setup: expected literal payload text in output, got:\n%s", got)
	}
	// The malicious payload must appear only inside an escaped label value,
	// never as a raw unescaped double-quote that could terminate the label
	// early and let the rest of the payload be interpreted as new metric
	// lines.
	if strings.Contains(got, `path="/evil"} extra_metric`) {
		t.Errorf("Render() did not escape untrusted path value, got:\n%s", got)
	}
}
