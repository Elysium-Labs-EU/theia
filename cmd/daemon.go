package cmd

import (
	"fmt"

	"github.com/Elysium-Labs-EU/theia/internal/ingest"
	"github.com/spf13/cobra"
)

// defaultDBPath and defaultLogPath are the shared flag defaults for every
// subcommand that takes --db-path/--log-path (daemon, serve, serve-metrics,
// stats) — diagnose.go also reuses them as its last-resort "default-guess"
// fallback. defaultDBPath is a fixed absolute path, not cwd-relative: theia
// always runs as a root-managed system service, so an absolute default is
// the one value every subcommand can converge on regardless of the caller's
// working directory — unlike a relative "./theia.db", which only happened to
// resolve correctly for the daemon because the systemd unit sets
// WorkingDirectory=/var/lib/theia.
const (
	defaultDBPath  = "/var/lib/theia/theia.db"
	defaultLogPath = "/var/log/nginx/access.log"
)

func newDaemonCmd() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Tail an nginx access log and write analytics to sqlite",
		Long: `daemon tails an nginx access log, parses each line into a page view,
and persists hourly aggregated stats to a sqlite database.

Example:
  theia daemon --log-path /var/log/nginx/access.log --db-path /var/lib/theia/theia.db`,

		RunE: func(cmd *cobra.Command, args []string) error {
			// Flags parsed fine to reach here, so any error from this point
			// on is a runtime failure, not a usage mistake — don't dump the
			// flags/usage block for it.
			cmd.SilenceUsage = true

			dbPath, err := cmd.Flags().GetString("db-path")
			if err != nil {
				return fmt.Errorf("parsing db-path flag: %w", err)
			}

			logPath, err := cmd.Flags().GetString("log-path")
			if err != nil {
				return fmt.Errorf("parsing log-path flag: %w", err)
			}

			return ingest.Run(cmd.Context(), dbPath, logPath)
		},
	}

	daemonCmd.Flags().String("db-path", defaultDBPath, "path to the sqlite database")
	daemonCmd.Flags().String("log-path", defaultLogPath, "path to the nginx access log")

	return daemonCmd
}
