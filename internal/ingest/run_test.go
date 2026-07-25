package ingest

import (
	"slices"
	"testing"
)

// TestBuildTailArgsStartsAtEndOfFile is a regression guard for issue #22: a
// daemon restart must not replay lines already in the log. "tail" defaults to
// "-n 10", so the "-n 0" here is what makes a fresh start follow only new
// appends instead of re-counting the last 10 existing lines on every restart.
func TestBuildTailArgsStartsAtEndOfFile(t *testing.T) {
	got := buildTailArgs("/var/log/access.log")
	want := []string{"-n", "0", "-F", "/var/log/access.log"}

	if !slices.Equal(got, want) {
		t.Fatalf("buildTailArgs = %q, want %q", got, want)
	}

	// Explicit "-n 0" is the load-bearing part; assert it precedes the path so
	// tail applies it as the follow start offset, not as a filename.
	if got[0] != "-n" || got[1] != "0" {
		t.Errorf("expected args to begin with -n 0 (start at EOF), got %q", got)
	}
}
