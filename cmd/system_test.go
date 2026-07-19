package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCmd(t *testing.T) {
	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"system", "version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if strings.TrimSpace(buf.String()) == "" {
		t.Error("expected non-empty version output")
	}
}

func TestSystemCmdHasSubcommands(t *testing.T) {
	want := []string{"version", "update", "uninstall"}
	system := newSystemCmd()
	for _, name := range want {
		found := false
		for _, c := range system.Commands() {
			if c.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("newSystemCmd() is missing subcommand %q", name)
		}
	}
}
