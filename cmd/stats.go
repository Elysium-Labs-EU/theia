package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"codeberg.org/Elysium_Labs/theia/database"
	"codeberg.org/Elysium_Labs/theia/internal/query"
)

//nolint:govet // fieldalignment: JSON output field order follows struct order; reordering would change the rendered output
type statsReport struct {
	Summary      query.Summary        `json:"summary"`
	TopPaths     []query.PathStat     `json:"top_paths"`
	StatusCodes  []query.StatusStat   `json:"status_codes"`
	TopReferrers []query.ReferrerStat `json:"top_referrers"`
}

func newStatsCmd() *cobra.Command {
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Query analytics from the sqlite database",
		Long: `stats reads page view analytics from the theia sqlite database.

Example:
  theia stats --db-path /var/lib/theia/theia.db
  theia stats --days 30 --host example.com --format json`,

		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, err := cmd.Flags().GetString("db-path")
			if err != nil {
				return fmt.Errorf("parsing db-path flag: %w", err)
			}
			days, err := cmd.Flags().GetInt("days")
			if err != nil {
				return fmt.Errorf("parsing days flag: %w", err)
			}
			host, err := cmd.Flags().GetString("host")
			if err != nil {
				return fmt.Errorf("parsing host flag: %w", err)
			}
			format, err := cmd.Flags().GetString("format")
			if err != nil {
				return fmt.Errorf("parsing format flag: %w", err)
			}
			top, err := cmd.Flags().GetInt("top")
			if err != nil {
				return fmt.Errorf("parsing top flag: %w", err)
			}

			return runStats(cmd, dbPath, days, host, format, top)
		},
	}

	statsCmd.Flags().String("db-path", "./theia.db", "path to the sqlite database")
	statsCmd.Flags().Int("days", 7, "number of days to look back")
	statsCmd.Flags().String("host", "", "filter by host (empty = all hosts)")
	statsCmd.Flags().String("format", "table", "output format: table or json")
	statsCmd.Flags().Int("top", 10, "number of top paths/referrers to show")

	return statsCmd
}

func runStats(cmd *cobra.Command, dbPath string, days int, host, format string, top int) error {
	db, err := database.Open(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	if err = database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	since := time.Now().AddDate(0, 0, -days)
	report, err := collectStats(cmd.Context(), db, since, host, top)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		return renderJSON(cmd, &report)
	default:
		return renderTable(cmd, &report, days, host)
	}
}

func collectStats(ctx context.Context, db *sql.DB, since time.Time, host string, top int) (statsReport, error) {
	summary, err := query.GetSummary(ctx, db, since, host)
	if err != nil {
		return statsReport{}, err
	}

	paths, err := query.GetTopPaths(ctx, db, since, host, top)
	if err != nil {
		return statsReport{}, err
	}

	statuses, err := query.GetStatusCodes(ctx, db, since, host)
	if err != nil {
		return statsReport{}, err
	}

	referrers, err := query.GetTopReferrers(ctx, db, since, host, top)
	if err != nil {
		return statsReport{}, err
	}

	return statsReport{
		Summary:      summary,
		TopPaths:     paths,
		StatusCodes:  statuses,
		TopReferrers: referrers,
	}, nil
}

func renderTable(cmd *cobra.Command, r *statsReport, days int, host string) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)

	period := fmt.Sprintf("last %d days", days)
	if host != "" {
		period += " - " + host
	}

	_, _ = fmt.Fprintf(w, "Summary (%s)\n", period)
	_, _ = fmt.Fprintf(w, "  Pageviews:\t%d\n", r.Summary.Pageviews)
	_, _ = fmt.Fprintf(w, "  Unique visitors:\t%d\n", r.Summary.UniqueVisitors)
	_, _ = fmt.Fprintf(w, "  Bot views:\t%d\n", r.Summary.BotViews)

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Top Paths")
	if len(r.TopPaths) == 0 {
		_, _ = fmt.Fprintln(w, "  (no data)")
	} else {
		_, _ = fmt.Fprintln(w, "  PATH\tHOST\tPAGEVIEWS")
		for _, p := range r.TopPaths {
			_, _ = fmt.Fprintf(w, "  %s\t%s\t%d\n", p.Path, p.Host, p.Pageviews)
		}
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Status Codes")
	if len(r.StatusCodes) == 0 {
		_, _ = fmt.Fprintln(w, "  (no data)")
	} else {
		_, _ = fmt.Fprintln(w, "  CODE\tCOUNT")
		for _, s := range r.StatusCodes {
			_, _ = fmt.Fprintf(w, "  %d\t%d\n", s.StatusCode, s.Count)
		}
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Top Referrers")
	if len(r.TopReferrers) == 0 {
		_, _ = fmt.Fprintln(w, "  (no data)")
	} else {
		_, _ = fmt.Fprintln(w, "  REFERRER\tCOUNT")
		for _, ref := range r.TopReferrers {
			_, _ = fmt.Fprintf(w, "  %s\t%d\n", ref.Referrer, ref.Count)
		}
	}

	return w.Flush()
}

func renderJSON(cmd *cobra.Command, r *statsReport) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
