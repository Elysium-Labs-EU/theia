package main

import (
	"os"
	"regexp"
	"testing"
)

// TestGoDirectiveVersion pins go.mod's go directive so a future toolchain
// bump requires touching this test, keeping issue #112-style upgrades visible.
func TestGoDirectiveVersion(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}

	re := regexp.MustCompile(`(?m)^go (\d+\.\d+\.\d+)$`)
	match := re.FindSubmatch(data)
	if match == nil {
		t.Fatal("go directive not found in go.mod")
	}

	const want = "1.27.0"
	if got := string(match[1]); got != want {
		t.Errorf("go.mod go directive = %q, want %q", got, want)
	}
}
