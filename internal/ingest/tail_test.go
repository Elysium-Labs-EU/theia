package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
