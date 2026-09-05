package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Elysium-Labs-EU/theia/internal/ui"
)

func TestRootCmd_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(buf.String(), "theia") {
		t.Errorf("expected version line in output\ngot: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "Available Commands") {
		t.Errorf("expected bare invocation to fall back to full help output\ngot: %s", buf.String())
	}
}

// TestRootCmd_RuntimeErrorSilencesUsage covers issue #18: a runtime failure
// (here, a corrupt sqlite file recognized only once "stats" starts running,
// not a flag-parsing mistake) must print just the error, not cobra's
// flags/usage block, and must not be printed twice.
func TestRootCmd_RuntimeErrorSilencesUsage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "garbage.db")
	if err := os.WriteFile(dbPath, []byte("not a sqlite db"), 0o600); err != nil {
		t.Fatalf("write garbage db: %v", err)
	}

	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"stats", "--db-path", dbPath})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for corrupt db file, got nil")
	}

	out := buf.String()
	if strings.Contains(out, "Usage:") || strings.Contains(out, "Flags:") {
		t.Errorf("runtime error should not print the usage/flags block\ngot: %s", out)
	}
	if got := strings.Count(out, err.Error()); got > 1 {
		t.Errorf("error message printed %d times by cobra itself, want caller to print it exactly once\ngot: %s", got, out)
	}
}

// TestRootCmd_FlagErrorShowsUsage ensures the fix for issue #18 doesn't
// over-silence: an actual flag-parsing mistake should still show usage.
func TestRootCmd_FlagErrorShowsUsage(t *testing.T) {
	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"stats", "--nonexistent-flag"})

	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}

	out := buf.String()
	if !strings.Contains(out, "Usage:") {
		t.Errorf("flag-parsing error should still print usage\ngot: %s", out)
	}
}

// A *ui.UserError's Hint is only useful if the top-level error printer
// actually renders it — Render() being defined is not enough on its own.
func TestFormatExecutionErrorRendersUserErrorHint(t *testing.T) {
	err := &ui.UserError{Err: errors.New("binary directory not writable"), Hint: "sudo theia system update"}

	got := formatExecutionError(err)

	if !strings.Contains(got, "sudo theia system update") {
		t.Errorf("formatExecutionError(%v) = %q, want it to contain the hint", err, got)
	}
	if !strings.Contains(got, "binary directory not writable") {
		t.Errorf("formatExecutionError(%v) = %q, want it to contain the message", err, got)
	}
}

// A *ui.UserError wrapped by an outer fmt.Errorf must still be recognized —
// callers are not required to return it unwrapped.
func TestFormatExecutionErrorRendersWrappedUserErrorHint(t *testing.T) {
	inner := &ui.UserError{Err: errors.New("binary directory not writable"), Hint: "sudo theia system update"}
	err := fmt.Errorf("installing update: %w", inner)

	got := formatExecutionError(err)

	if !strings.Contains(got, "sudo theia system update") {
		t.Errorf("formatExecutionError(%v) = %q, want it to contain the hint", err, got)
	}
}

// A plain error (no hint to offer) must render exactly as before: its own
// Error() text, nothing more.
func TestFormatExecutionErrorPlainError(t *testing.T) {
	err := errors.New("something went wrong")

	got := formatExecutionError(err)

	if got != err.Error() {
		t.Errorf("formatExecutionError(%v) = %q, want %q", err, got, err.Error())
	}
}
