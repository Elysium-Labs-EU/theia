// Package cmd implements the theia CLI commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"theia/internal/buildinfo"
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "theia",
		Short: "Privacy-first server-side analytics",
		Long: fmt.Sprintf(`theia %s

theia tracks page views by parsing nginx access logs.
No client-side JavaScript required, making it resistant to ad-blockers.`, buildinfo.GetVersionOnly()),
	}

	rootCmd.AddCommand(newDaemonCmd())
	rootCmd.AddCommand(newStatsCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newCompletionCmd(rootCmd))

	return rootCmd
}

// Execute is the entry point for the theia CLI.
// It builds the root command tree and exits with code 1 on error.
func Execute() {
	rootCmd := newRootCmd()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
