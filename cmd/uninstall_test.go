package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFakeBinary(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "theia")
	if err := os.WriteFile(path, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestRunUninstallDeclinedLeavesEverything(t *testing.T) {
	dir := t.TempDir()
	exePath := writeFakeBinary(t, dir)
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := runUninstall(context.Background(), strings.NewReader("n\n"), buf, exePath, dataDir, false, false); err != nil {
		t.Fatalf("runUninstall: %v", err)
	}

	if _, err := os.Stat(exePath); err != nil {
		t.Errorf("expected binary to survive a declined confirmation, stat err: %v", err)
	}
}

func TestRunUninstallYesRemovesBinaryLeavesData(t *testing.T) {
	dir := t.TempDir()
	exePath := writeFakeBinary(t, dir)
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := runUninstall(context.Background(), strings.NewReader(""), buf, exePath, dataDir, true, false); err != nil {
		t.Fatalf("runUninstall: %v", err)
	}

	if _, err := os.Stat(exePath); !os.IsNotExist(err) {
		t.Errorf("expected binary to be removed, stat err: %v", err)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Errorf("expected data dir to survive without --purge, stat err: %v", err)
	}
	if !strings.Contains(buf.String(), "rm -rf") {
		t.Errorf("output = %q, want a manual-cleanup hint", buf.String())
	}
}

func TestRunUninstallPurgeRemovesData(t *testing.T) {
	dir := t.TempDir()
	exePath := writeFakeBinary(t, dir)
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := runUninstall(context.Background(), strings.NewReader(""), buf, exePath, dataDir, true, true); err != nil {
		t.Fatalf("runUninstall: %v", err)
	}

	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Errorf("expected data dir to be removed with --purge, stat err: %v", err)
	}
}

func TestRunUninstallMissingBinaryIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "already-gone")
	dataDir := filepath.Join(dir, "data")

	buf := &bytes.Buffer{}
	if err := runUninstall(context.Background(), strings.NewReader(""), buf, exePath, dataDir, true, false); err != nil {
		t.Fatalf("runUninstall on missing binary: %v", err)
	}
}
