package cmd

import (
	"fmt"

	"github.com/Elysium-Labs-EU/theia/database"
	"github.com/Elysium-Labs-EU/theia/internal/apiserver"
	"github.com/Elysium-Labs-EU/theia/internal/promsink"
	"github.com/spf13/cobra"
)

func newServeMetricsCmd() *cobra.Command {
	serveMetricsCmd := &cobra.Command{
		Use:   "serve-metrics",
		Short: "Serve pageview, status-code, and referrer counts as Prometheus metrics",
		Long: `serve-metrics exposes theia's analytics as standard Prometheus counters on a
GET /metrics endpoint, independent of the bearer-authed stats API that
"theia serve" runs.

Point a Prometheus scrape_config at this endpoint instead of writing a
custom scraper against the JSON/CSV stats API.

It binds to 127.0.0.1 only — put it behind a reverse proxy (e.g. nginx)
to expose it beyond localhost.

Example:
  theia serve-metrics --db-path /var/lib/theia/theia.db --addr 127.0.0.1:8082`,

		RunE: func(cmd *cobra.Command, args []string) error {
			// Flags parsed fine to reach here, so any error from this point
			// on is a runtime failure, not a usage mistake — don't dump the
			// flags/usage block for it.
			cmd.SilenceUsage = true

			dbPath, err := cmd.Flags().GetString("db-path")
			if err != nil {
				return fmt.Errorf("parsing db-path flag: %w", err)
			}
			addr, err := cmd.Flags().GetString("addr")
			if err != nil {
				return fmt.Errorf("parsing addr flag: %w", err)
			}
			top, err := cmd.Flags().GetInt("top")
			if err != nil {
				return fmt.Errorf("parsing top flag: %w", err)
			}
			if top <= 0 {
				return fmt.Errorf("invalid --top %d: must be a positive integer", top)
			}

			if err := apiserver.ValidateLoopbackAddr(addr); err != nil {
				return err
			}

			return runServeMetrics(cmd, dbPath, addr, top)
		},
	}

	serveMetricsCmd.Flags().String("db-path", "./theia.db", "path to the sqlite database")
	serveMetricsCmd.Flags().String("addr", "127.0.0.1:8082", "address to bind the metrics endpoint to (must be 127.0.0.1 or localhost)")
	serveMetricsCmd.Flags().Int("top", 20, "max number of distinct paths/referrers exported (bounds Prometheus label cardinality)")

	return serveMetricsCmd
}

func runServeMetrics(cmd *cobra.Command, dbPath, addr string, top int) error {
	db, err := database.Open(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	if err := database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	cmd.Printf("Metrics endpoint listening on %s\n", addr)
	return promsink.Run(cmd.Context(), db, promsink.Config{Addr: addr, Top: top})
}
