package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDaemonCmd_StopsOnContextCancellation is a regression test for #14:
// the daemon caught SIGTERM/SIGINT, logged that it was stopping, but never
// actually exited because no cancellable context reached the "tail -f"
// child process. It exercises the real command tree (the same RunE that
// Execute wires cmd.Context() into) so a future regression here — in
// daemon.go, ingest.Run, or tailLog's exec.CommandContext usage — fails
// this test instead of silently reintroducing the hang.
func TestDaemonCmd_StopsOnContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "theia.db")
	logPath := filepath.Join(tempDir, "access.log")

	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatalf("failed to create log file: %v", err)
	}

	daemonCmd := newDaemonCmd()
	daemonCmd.SetArgs([]string{"--db-path", dbPath, "--log-path", logPath})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- daemonCmd.ExecuteContext(ctx)
	}()

	// Give "tail -f" time to start before simulating the shutdown signal.
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("daemon command returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not stop within 5s of context cancellation (regression of #14: SIGTERM/SIGINT never actually shuts down the daemon)")
	}
}

// TestDaemonCmd_ReturnsErrorForMissingLogFile is a regression test for #15:
// the daemon used to return nil (and so, via cmd/root.go's Execute, exit 0)
// when given a --log-path that doesn't exist, because "tail -F" retries
// forever without exiting and its diagnostic was discarded. The command must
// now return a non-nil error so the process exits non-zero with a clear
// message.
func TestDaemonCmd_ReturnsErrorForMissingLogFile(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "theia.db")
	logPath := filepath.Join(tempDir, "does-not-exist.log")

	daemonCmd := newDaemonCmd()
	daemonCmd.SetArgs([]string{"--db-path", dbPath, "--log-path", logPath})

	done := make(chan error, 1)
	go func() {
		done <- daemonCmd.ExecuteContext(context.Background())
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected daemon command to return an error for a missing log file, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not return within 5s for a missing log file (regression of #15: tail -F retries forever instead of failing fast)")
	}
}
