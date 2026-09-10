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

Matching lines are parsed but never written to the database — permanent,
unlike stats's --exclude-host/--exclude-path, which only hide existing data
from reports.

Example:
  theia daemon --log-path /var/log/nginx/access.log --db-path /var/lib/theia/theia.db
  theia daemon --exclude-host navidrome.home.rtgs.me --exclude-path /rest/ping`,

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

			filter, err := daemonFilterFromFlags(cmd)
			if err != nil {
				return err
			}

			return ingest.Run(cmd.Context(), dbPath, logPath, filter)
		},
	}

	daemonCmd.Flags().String("db-path", defaultDBPath, "path to the sqlite database")
	daemonCmd.Flags().String("log-path", defaultLogPath, "path to the nginx access log")
	daemonCmd.Flags().StringSlice("include-host", nil, "only store page views for this host (repeatable; empty = all hosts)")
	daemonCmd.Flags().StringSlice("exclude-host", nil, "never store page views for this host (repeatable; wins over --include-host)")
	daemonCmd.Flags().StringSlice("exclude-path", nil, "never store page views whose path starts with this prefix (repeatable)")

	return daemonCmd
}

// daemonFilterFromFlags reads daemon's --include-host/--exclude-host/
// --exclude-path flags into an ingest.Filter.
func daemonFilterFromFlags(cmd *cobra.Command) (ingest.Filter, error) {
	includeHosts, err := cmd.Flags().GetStringSlice("include-host")
	if err != nil {
		return ingest.Filter{}, fmt.Errorf("parsing include-host flag: %w", err)
	}
	excludeHosts, err := cmd.Flags().GetStringSlice("exclude-host")
	if err != nil {
		return ingest.Filter{}, fmt.Errorf("parsing exclude-host flag: %w", err)
	}
	excludePaths, err := cmd.Flags().GetStringSlice("exclude-path")
	if err != nil {
		return ingest.Filter{}, fmt.Errorf("parsing exclude-path flag: %w", err)
	}
	return ingest.NewFilter(includeHosts, excludeHosts, excludePaths), nil
}
