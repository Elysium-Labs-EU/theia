package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultYes bool
		want       bool
	}{
		{"explicit yes", "y\n", false, true},
		{"explicit yes long form", "yes\n", false, true},
		{"explicit no overrides default yes", "n\n", true, false},
		{"empty response uses default (yes)", "\n", true, true},
		{"empty response uses default (no)", "\n", false, false},
		{"case insensitive", "Y\n", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			got := Confirm(in, out, "proceed?", tt.defaultYes)
			if got != tt.want {
				t.Errorf("Confirm(%q, defaultYes=%v) = %v, want %v", tt.input, tt.defaultYes, got, tt.want)
			}
			if out.Len() == 0 {
				t.Error("expected a prompt to be written to out")
			}
		})
	}
}
