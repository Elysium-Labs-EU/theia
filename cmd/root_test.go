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

	if !strings.Contains(buf.String(), "theia") {
		t.Errorf("expected version line in output\ngot: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "Available Commands") {
		t.Errorf("expected bare invocation to fall back to full help output\ngot: %s", buf.String())
	}
}
