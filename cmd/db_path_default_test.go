package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// Every subcommand that takes --db-path must default it to defaultDBPath, so
// omitting the flag always resolves to the same file regardless of which
// subcommand runs it or what directory it's invoked from. A hardcoded
// literal in just one command (e.g. "./theia.db") would silently reintroduce
// the cwd-dependent divergence this guards against.
func TestDBPathFlagDefaultsMatchSharedConstant(t *testing.T) {
	constructors := map[string]func() *cobra.Command{
		"daemon":        newDaemonCmd,
		"serve":         newServeCmd,
		"serve-metrics": newServeMetricsCmd,
		"stats":         newStatsCmd,
	}

	for name, newCmd := range constructors {
		t.Run(name, func(t *testing.T) {
			flag := newCmd().Flags().Lookup("db-path")
			if flag == nil {
				t.Fatalf("%s has no --db-path flag", name)
			}
			if flag.DefValue != defaultDBPath {
				t.Errorf("%s --db-path default = %q, want %q", name, flag.DefValue, defaultDBPath)
			}
		})
	}
}
