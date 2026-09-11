package ingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTailLogFollowsRenameBasedRotation guards against regressing to plain
// `tail -f`, which follows the open file descriptor and silently stops
// ingesting once logrotate renames the current log and creates a fresh file
// at the same path (the default Ubuntu/nginx logrotate behavior). `-F`
// follows the path instead, so it must pick up lines written after the
// rename.
func TestTailLogFollowsRenameBasedRotation(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "access.log")

	if err := os.WriteFile(logPath, []byte(accessLogLine("/a")+"\n"), 0o600); err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	pageViews := make(chan PageView, 10)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := tailLog(ctx, []string{"-F", logPath}, pageViews, Filter{}); err != nil {
			t.Errorf("tailLog returned unexpected error: %v", err)
		}
	}()

	first := waitForPageView(t, pageViews)
	if first.Path != "/a" {
		t.Fatalf("expected first page view path /a, got %s", first.Path)
	}

	if err := os.Rename(logPath, logPath+".1"); err != nil {
		t.Fatalf("failed to rename log file: %v", err)
	}
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatalf("failed to recreate log file at original path: %v", err)
	}
	if err := appendAccessLogLine(logPath, "/b"); err != nil {
		t.Fatalf("failed to append to rotated log file: %v", err)
	}

	second := waitForPageView(t, pageViews)
	if second.Path != "/b" {
		t.Fatalf("expected page view path /b after rotation, got %s", second.Path)
	}

	cancel()
	<-done
}

// TestTailLog_ReturnsErrorWithStderrForInvalidArgs is a regression test for
// #15: tailLogCommand.Stderr was never wired up, so when the "tail" process
// exits on its own (as opposed to being killed by ctx cancellation) its
// diagnostic was silently discarded and tailLog returned nothing. tailLog
// must now return an error that carries tail's stderr output.
func TestTailLog_ReturnsErrorWithStderrForInvalidArgs(t *testing.T) {
	pageViews := make(chan PageView, 1)

	err := tailLog(t.Context(), []string{"--this-flag-does-not-exist"}, pageViews, Filter{})
	if err == nil {
		t.Fatal("expected tailLog to return an error for an invalid tail argument, got nil")
	}
	if !strings.Contains(err.Error(), "this-flag-does-not-exist") {
		t.Errorf("expected error to carry tail's stderr diagnostic, got: %v", err)
	}
}

// TestTailLogAppliesFilter is an end-to-end check that a Filter passed to
// tailLog actually stops matching lines from ever reaching the pageViews
// channel — not just that allowPageView returns the right bool in isolation.
func TestTailLogAppliesFilter(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "access.log")

	lines := strings.Join([]string{
		accessLogLineWithHost("/rest/ping", "navidrome.home.rtgs.me"),
		accessLogLineWithHost("/", "elysiumlabs.dev"),
		accessLogLineWithHost("/keep", "navidrome.home.rtgs.me"),
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(lines), 0o600); err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	pageViews := make(chan PageView, 10)
	done := make(chan struct{})
	filter := NewFilter(nil, []string{"navidrome.home.rtgs.me"}, nil)
	go func() {
		defer close(done)
		if err := tailLog(ctx, []string{"-F", logPath}, pageViews, filter); err != nil {
			t.Errorf("tailLog returned unexpected error: %v", err)
		}
	}()

	got := waitForPageView(t, pageViews)
	if got.Host != "elysiumlabs.dev" || got.Path != "/" {
		t.Fatalf("expected the one non-excluded host, got %+v", got)
	}

	select {
	case extra := <-pageViews:
		t.Fatalf("expected no further page views (excluded host lines should be dropped), got %+v", extra)
	case <-time.After(200 * time.Millisecond):
	}

	cancel()
	<-done
}

func waitForPageView(t *testing.T, pageViews <-chan PageView) PageView {
	t.Helper()
	select {
	case pv := <-pageViews:
		return pv
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for page view")
		return PageView{}
	}
}

func accessLogLine(path string) string {
	return `127.0.0.1 - - [20/Jul/2026:10:00:00 +0000] "GET ` + path + ` HTTP/1.1" 200 100 "-" "Mozilla/5.0"`
}

func accessLogLineWithHost(path, host string) string {
	return accessLogLine(path) + ` "` + host + `"`
}

func appendAccessLogLine(path, urlPath string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // close error in defer is not actionable

	_, err = file.WriteString(accessLogLine(urlPath) + "\n")
	return err
}
