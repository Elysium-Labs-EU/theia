package ingest

import (
	"context"
	"os"
	"path/filepath"
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
		tailLog(ctx, []string{"-F", logPath}, pageViews)
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

func appendAccessLogLine(path, urlPath string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck // close error in defer is not actionable

	_, err = file.WriteString(accessLogLine(urlPath) + "\n")
	return err
}
