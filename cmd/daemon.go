package cmd

import (
	"fmt"

	"github.com/Elysium-Labs-EU/theia/internal/ingest"
	"github.com/spf13/cobra"
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

	daemonCmd.Flags().String("db-path", "./theia.db", "path to the sqlite database")
	daemonCmd.Flags().String("log-path", "/var/log/nginx/access.log", "path to the nginx access log")

	return daemonCmd
}
