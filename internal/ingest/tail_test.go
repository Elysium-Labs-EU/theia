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
		if err := tailLog(ctx, []string{"-F", logPath}, pageViews); err != nil {
			t.Errorf("tailLog: %v", err)
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

// TestTailLog_MissingFile verifies that tailLog surfaces a non-nil error
// (instead of silently returning nil) when the target log file does not
// exist, so callers can fail loudly rather than exiting 0 with no data.
func TestTailLog_MissingFile(t *testing.T) {
	pageViews := make(chan PageView, 1)
	logPath := filepath.Join(t.TempDir(), "does-not-exist.log")

	err := tailLog(t.Context(), []string{"-n", "+1", logPath}, pageViews)
	if err == nil {
		t.Fatal("expected an error for a missing log file, got nil")
	}
}

// TestTailLog_PermissionDenied verifies that tailLog surfaces a non-nil
// error when the log file exists but is not readable by the current user.
func TestTailLog_PermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which bypasses file permission checks")
	}

	logPath := filepath.Join(t.TempDir(), "unreadable.log")
	if err := os.WriteFile(logPath, []byte("line\n"), 0o000); err != nil {
		t.Fatalf("creating unreadable log file: %v", err)
	}

	pageViews := make(chan PageView, 1)
	err := tailLog(t.Context(), []string{"-n", "+1", logPath}, pageViews)
	if err == nil {
		t.Fatal("expected an error for an unreadable log file, got nil")
	}
}

// TestTailLog_Success verifies that tailLog still returns nil and delivers
// page views for a readable, well-formed log file.
func TestTailLog_Success(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "access.log")
	line := `127.0.0.1 - - [10/Oct/2023:13:55:36 +0000] "GET /index.html HTTP/1.1" 200 512 "-" "Mozilla/5.0"` + "\n"
	if err := os.WriteFile(logPath, []byte(line), 0o600); err != nil {
		t.Fatalf("creating log file: %v", err)
	}

	pageViews := make(chan PageView, 1)
	err := tailLog(t.Context(), []string{"-n", "+1", logPath}, pageViews)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	select {
	case pv := <-pageViews:
		if pv.Path != "/index.html" {
			t.Fatalf("expected path /index.html, got %q", pv.Path)
		}
	default:
		t.Fatal("expected a page view to be delivered")
	}
}

// TestTailLog_ErrorMentionsCause verifies the returned error carries the
// underlying tail diagnostic instead of a generic message, so operators can
// tell missing-file apart from permission-denied at a glance.
func TestTailLog_ErrorMentionsCause(t *testing.T) {
	pageViews := make(chan PageView, 1)
	logPath := filepath.Join(t.TempDir(), "does-not-exist.log")

	err := tailLog(t.Context(), []string{"-n", "+1", logPath}, pageViews)
	if err == nil {
		t.Fatal("expected an error for a missing log file, got nil")
	}
	if !strings.Contains(err.Error(), "tail command failed") {
		t.Fatalf("expected error to mention the tail command failure, got: %v", err)
	}
}

// TestTailLog_ContextCancellationIsNotAnError verifies that canceling ctx
// (the caller-requested-shutdown path exercised by Run) returns a nil error,
// even though it kills the "tail" child the same way a real tail failure
// would. Without this, every ordinary shutdown would surface as a spurious
// error from Run.
func TestTailLog_ContextCancellationIsNotAnError(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "access.log")
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatalf("creating log file: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	pageViews := make(chan PageView, 1)

	done := make(chan error, 1)
	go func() {
		done <- tailLog(ctx, []string{"-F", logPath}, pageViews)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on context cancellation, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("tailLog did not return within 5s of context cancellation")
	}
}
