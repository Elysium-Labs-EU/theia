package ingest

import "testing"

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"Example.com":     "example.com",
		"EXAMPLE.COM":     "example.com",
		"example.com":     "example.com",
		"":                "",
		"Sub.Example.COM": "sub.example.com",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// nginx logs a truly empty request line (e.g. a TLS probe or truncated
// request hitting an HTTP listener) as `""` in the combined log format. That
// still carries a real status code and byte count, so it must be recorded
// rather than dropped outright.
func TestParseNginxLogHandlesEmptyRequestLine(t *testing.T) {
	line := `127.0.0.1 - - [20/Jul/2026:10:00:00 +0000] "" 400 0 "-" "-"`
	pv, err := parseNginxLog(line)
	if err != nil {
		t.Fatalf("parseNginxLog(%q) error: %v", line, err)
	}
	if pv.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", pv.StatusCode)
	}
	if pv.Path != "" {
		t.Errorf("Path = %q, want empty", pv.Path)
	}
}

// The same empty-request-line case with a trailing Host field (regexWithHost).
func TestParseNginxLogHandlesEmptyRequestLineWithHost(t *testing.T) {
	line := `127.0.0.1 - - [20/Jul/2026:10:00:00 +0000] "" 400 0 "-" "-" "example.com"`
	pv, err := parseNginxLog(line)
	if err != nil {
		t.Fatalf("parseNginxLog(%q) error: %v", line, err)
	}
	if pv.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", pv.StatusCode)
	}
	if pv.Host != "example.com" {
		t.Errorf("Host = %q, want %q", pv.Host, "example.com")
	}
}

// Case variants of the same Host must parse into one lowercased bucket so
// per-host aggregation and --host filtering do not silently fragment (issue #26).
func TestParseNginxLogLowercasesHost(t *testing.T) {
	variants := []string{"Example.com", "example.com", "EXAMPLE.COM"}
	for _, h := range variants {
		line := `127.0.0.1 - - [20/Jul/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 512 "-" "Mozilla/5.0" "` + h + `"`
		pv, err := parseNginxLog(line)
		if err != nil {
			t.Fatalf("parseNginxLog(%q) error: %v", h, err)
		}
		if pv.Host != "example.com" {
			t.Errorf("Host for %q = %q, want %q", h, pv.Host, "example.com")
		}
	}
}
