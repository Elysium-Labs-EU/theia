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
		Long: `theia tracks page views by parsing nginx access logs.
No client-side JavaScript required, making it resistant to ad-blockers.`,

		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("theia %s\n\n", buildinfo.GetVersionOnly())
			cmd.Printf("note: %s -> see available commands\n\n", "theia help")
		},
	}

	rootCmd.AddCommand(newDaemonCmd())
	rootCmd.AddCommand(newStatsCmd())
	rootCmd.AddCommand(newVersionCmd())

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
