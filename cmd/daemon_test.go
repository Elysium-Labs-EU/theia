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
