// Package cmd implements the theia CLI commands.
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Elysium-Labs-EU/theia/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "theia",
		Short: "Privacy-first server-side analytics",
		Long: fmt.Sprintf(`theia %s

theia tracks page views by parsing nginx access logs.
No client-side JavaScript required, making it resistant to ad-blockers.`, buildinfo.GetVersionOnly()),
		Version: buildinfo.Get(),
	}
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	rootCmd.AddCommand(newDaemonCmd())
	rootCmd.AddCommand(newStatsCmd())
	rootCmd.AddCommand(newSystemCmd())
	rootCmd.AddCommand(newCompletionCmd(rootCmd))

	return rootCmd
}

// Execute is the entry point for the theia CLI.
// It builds the root command tree, wires a context that's canceled on
// SIGINT/SIGTERM so long-running commands (e.g. daemon) can shut down
// cleanly, and exits with code 1 on error.
func Execute() {
	rootCmd := newRootCmd()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	err := rootCmd.ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
