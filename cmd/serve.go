package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Elysium-Labs-EU/theia/database"
	"github.com/Elysium-Labs-EU/theia/internal/apiserver"
	"github.com/spf13/cobra"
)

// theiaAPITokenEnv is the env var fallback for the bearer token when
// neither --token nor --token-file is set, mirroring THEIA_DEFAULT_HOST's
// role for the daemon's host default.
const theiaAPITokenEnv = "THEIA_API_TOKEN"

func newServeCmd() *cobra.Command {
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the stats API over HTTP",
		Long: `serve exposes theia's analytics as a bearer-authed, filterable read API
(JSON or CSV) over the same sqlite database the daemon writes to.

It binds to 127.0.0.1 only — put it behind a reverse proxy (e.g. nginx)
to expose it beyond localhost.

Example:
  theia serve --db-path /var/lib/theia/theia.db --token-file /etc/theia/api-token`,

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
			token, err := cmd.Flags().GetString("token")
			if err != nil {
				return fmt.Errorf("parsing token flag: %w", err)
			}
			tokenFile, err := cmd.Flags().GetString("token-file")
			if err != nil {
				return fmt.Errorf("parsing token-file flag: %w", err)
			}

			resolvedToken, err := resolveAPIToken(token, tokenFile)
			if err != nil {
				return err
			}

			if err := apiserver.ValidateLoopbackAddr(addr); err != nil {
				return err
			}

			return runServe(cmd, dbPath, addr, resolvedToken)
		},
	}

	serveCmd.Flags().String("db-path", defaultDBPath, "path to the sqlite database")
	serveCmd.Flags().String("addr", "127.0.0.1:8081", "address to bind the stats API to (must be 127.0.0.1 or localhost)")
	serveCmd.Flags().String("token", "", "bearer token required on every request (avoid on shared machines — visible in the process list; prefer --token-file or "+theiaAPITokenEnv)
	serveCmd.Flags().String("token-file", "", "path to a file containing the bearer token")

	return serveCmd
}

// resolveAPIToken picks the bearer token from, in priority order, the
// --token flag, the --token-file file, then the THEIA_API_TOKEN env var —
// erroring if none configured, since an API with no token would otherwise
// serve every visitor's page-view history to anyone on localhost.
func resolveAPIToken(token, tokenFile string) (string, error) {
	if token != "" {
		return token, nil
	}

	if tokenFile != "" {
		contents, err := os.ReadFile(tokenFile) //nolint:gosec // path comes from an operator-supplied CLI flag, not request input
		if err != nil {
			return "", fmt.Errorf("reading token file %q: %w", tokenFile, err)
		}
		fileToken := strings.TrimSpace(string(contents))
		if fileToken == "" {
			return "", fmt.Errorf("token file %q is empty", tokenFile)
		}
		return fileToken, nil
	}

	if envToken := os.Getenv(theiaAPITokenEnv); envToken != "" {
		return envToken, nil
	}

	return "", fmt.Errorf("no bearer token configured: set --token, --token-file, or %s", theiaAPITokenEnv)
}

func runServe(cmd *cobra.Command, dbPath, addr, token string) error {
	db, err := database.Open(cmd.Context(), dbPath)
	if err != nil {
		return err
	}
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	if err := database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	cmd.Printf("Stats API listening on %s\n", addr)
	return apiserver.Run(cmd.Context(), db, apiserver.Config{Addr: addr, Token: token})
}
