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
