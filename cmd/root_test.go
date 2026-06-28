package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_NoArgs(t *testing.T) {
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(buf.String(), "theia help") {
		t.Errorf("expected 'theia help' in output\ngot: %s", buf.String())
	}
}
