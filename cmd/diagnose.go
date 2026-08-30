package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
	"github.com/Elysium-Labs-EU/theia/internal/buildinfo"
	"github.com/Elysium-Labs-EU/theia/internal/ui"
	"github.com/spf13/cobra"
)

// diagnoseOptions carries the resolved --db-path/--log-path/--since/--lines/
// --output/--no-logs flag values through to runDiagnose.
type diagnoseOptions struct {
	DBPath  string
	LogPath string
	Output  string
	Since   time.Duration
	Lines   int
	NoLogs  bool
}

// diagnoseManifest is the top-level manifest.json entry in the bundle: what
// was collected and what failed. GeneratedAt/HostID/OS/Arch/step names are
// allowlisted fixed-shape fields. A step's Error/Note text is free-form —
// it comes straight from Go's own error messages — so it is run through
// diagnoseScrubLine before being stored (see diagnoseAppendStep) the same as
// daemon.log's lines are.
type diagnoseManifest struct {
	GeneratedAt time.Time            `json:"generated_at"`
	HostID      string               `json:"host_id"`
	OS          string               `json:"os"`
	Arch        string               `json:"arch"`
	Steps       []diagnoseStepResult `json:"steps"`
}

// diagnoseStepResult records one independent collection step's outcome, so a
// single failure (daemon down, missing db file, unreadable log) never
// prevents the rest of the bundle from being produced.
type diagnoseStepResult struct {
	Name     string `json:"name"`
	Error    string `json:"error,omitempty"`
	Note     string `json:"note,omitempty"`
	Captured bool   `json:"captured"`
}

// diagnoseFile is one file's content staged for the output archive.
type diagnoseFile struct {
	Name string
	Data []byte
}

// diagnoseDaemonInfo is daemon.json: whether theia.service is active under
// systemd and its pid, resolved via the same read-only systemctl probes
// serviceIsActive already uses. Never starts or stops the daemon.
type diagnoseDaemonInfo struct {
	Pid    *int `json:"pid,omitempty"`
	Active bool `json:"active"`
}

// diagnoseConfigInfo is config.json: the daemon's actual --db-path/--log-path,
// and where that value came from. ResolvedFrom is "daemon-process" when read
// from the running daemon's own /proc/<pid>/cmdline — the one source that
// can't be stale or guessed — falling back to "flag" (explicit --db-path/
// --log-path on this command) or "default-guess" (theia's own CLI default,
// unverified against anything actually running).
//
//nolint:govet // fieldalignment: JSON output field order follows struct order; reordering would change the rendered output
type diagnoseConfigInfo struct {
	DBPath          string `json:"db_path"`
	DBPathSource    string `json:"db_path_source"`
	DBPathExists    bool   `json:"db_path_exists"`
	LogPath         string `json:"log_path"`
	LogPathSource   string `json:"log_path_source"`
	LogPathReadable bool   `json:"log_path_readable"`
}

// diagnoseTableInfo is one table's entry in db.json.
//
//nolint:govet // fieldalignment: JSON output field order follows struct order; reordering would change the rendered output
type diagnoseTableInfo struct {
	Rows       int64  `json:"rows"`
	MostRecent string `json:"most_recent,omitempty"`
}

// diagnoseDBInfo is db.json: row counts and freshness per table in the
// resolved sqlite database, plus its migration version. This is the piece
// that answers "why is `theia stats` empty" directly: an operator can see
// whether the database diagnose opened actually has any rows, without first
// having to guess whether it's even the same file the daemon writes to.
//
//nolint:govet // fieldalignment: JSON output field order follows struct order; reordering would change the rendered output
type diagnoseDBInfo struct {
	SizeBytes        int64                        `json:"size_bytes"`
	Tables           map[string]diagnoseTableInfo `json:"tables"`
	MigrationVersion uint                         `json:"migration_version"`
	MigrationDirty   bool                         `json:"migration_dirty"`
}

func newDiagnoseCmd() *cobra.Command {
	diagnoseCmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Bundle a privacy-safe diagnostic archive for bug reports",
		Long: `diagnose collects version, daemon status, the resolved database's row
counts, and recent daemon logs into a single tar.gz suitable for attaching
to a public GitHub issue.

Every collection step is recorded independently in the bundle's
manifest.json as captured or failed; a daemon that's down or a missing
database file never prevents a bundle from being produced. Only a failure
to write the output file itself aborts the command.

Without --db-path/--log-path, diagnose reads the actually-running daemon's
own flags from its process (/proc/<pid>/cmdline) rather than guessing —
this is the piece "theia stats" can't tell you: whether the database you're
querying is the one the daemon is actually writing to.

Example:
  theia diagnose
  theia diagnose --since 30m --lines 2000
  theia diagnose --output /tmp/bug-report.tar.gz`,

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
			output, err := cmd.Flags().GetString("output")
			if err != nil {
				return fmt.Errorf("parsing output flag: %w", err)
			}
			since, err := cmd.Flags().GetDuration("since")
			if err != nil {
				return fmt.Errorf("parsing since flag: %w", err)
			}
			lines, err := cmd.Flags().GetInt("lines")
			if err != nil {
				return fmt.Errorf("parsing lines flag: %w", err)
			}
			noLogs, err := cmd.Flags().GetBool("no-logs")
			if err != nil {
				return fmt.Errorf("parsing no-logs flag: %w", err)
			}

			return runDiagnose(cmd, diagnoseOptions{
				DBPath:  dbPath,
				LogPath: logPath,
				Output:  output,
				Since:   since,
				Lines:   lines,
				NoLogs:  noLogs,
			})
		},
	}

	diagnoseCmd.Flags().String("db-path", "", "override the resolved db-path (default: read from the running daemon, else ./theia.db)")
	diagnoseCmd.Flags().String("log-path", "", "override the resolved log-path (default: read from the running daemon, else /var/log/nginx/access.log)")
	diagnoseCmd.Flags().String("output", "", "output tar.gz path (default ./theia-diagnose-<timestamp>.tar.gz)")
	diagnoseCmd.Flags().Duration("since", 10*time.Minute, "time window for the daemon log")
	diagnoseCmd.Flags().Int("lines", 1000, "hard cap on log lines collected")
	diagnoseCmd.Flags().Bool("no-logs", false, "skip the daemon log (journalctl)")

	return diagnoseCmd
}

func runDiagnose(cmd *cobra.Command, opts diagnoseOptions) error {
	manifest, files := diagnoseCollect(cmd.Context(), opts)

	outputPath := opts.Output
	if outputPath == "" {
		outputPath = fmt.Sprintf("theia-diagnose-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	}

	if err := diagnoseWriteBundle(outputPath, manifest, files); err != nil {
		return fmt.Errorf("writing diagnostic bundle: %w", err)
	}

	cmd.Printf("%s wrote diagnostic bundle to %s\n", ui.LabelSuccess.Render("✓"), ui.TextBold.Render(outputPath))
	cmd.Printf("  %s attach it to a new issue at https://github.com/Elysium-Labs-EU/theia/issues/new\n", ui.TextMuted.Render("next:"))

	failed := 0
	for _, step := range manifest.Steps {
		if !step.Captured {
			failed++
		}
	}
	if failed > 0 {
		cmd.Printf("  %s %d of %d collection steps failed (see manifest.json in the bundle) — the bundle was still produced\n",
			ui.TextMuted.Render("note:"), failed, len(manifest.Steps))
	}
	return nil
}

// diagnoseCollect runs every collection step and returns the assembled
// manifest plus every collected file's content. No individual step's
// failure stops another from running.
func diagnoseCollect(ctx context.Context, opts diagnoseOptions) (*diagnoseManifest, []diagnoseFile) {
	manifest := &diagnoseManifest{
		GeneratedAt: time.Now().UTC(),
		HostID:      diagnoseHostID(),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
	}
	var files []diagnoseFile

	diagnoseAppendStep(manifest, diagnoseStepResult{Name: "version", Captured: true})
	files = append(files, diagnoseJSONFile("version.json", buildinfo.Get()))

	daemonInfo, daemonStep := diagnoseCollectDaemonInfo(ctx)
	diagnoseAppendStep(manifest, daemonStep)
	files = append(files, diagnoseJSONFile("daemon.json", daemonInfo))

	cmdline, cmdlineStep := diagnoseReadDaemonCmdline(ctx, daemonInfo.Pid)
	diagnoseAppendStep(manifest, cmdlineStep)

	configInfo := diagnoseResolveConfig(opts, cmdline)
	diagnoseAppendStep(manifest, diagnoseStepResult{Name: "config", Captured: true})
	files = append(files, diagnoseJSONFile("config.json", configInfo))

	dbInfo, dbStep := diagnoseCollectDB(ctx, configInfo.DBPath, configInfo.DBPathExists)
	diagnoseAppendStep(manifest, dbStep)
	if dbInfo != nil {
		files = append(files, diagnoseJSONFile("db.json", dbInfo))
	}

	if !opts.NoLogs {
		logFile, logStep := diagnoseCollectDaemonLog(ctx, opts)
		diagnoseAppendStep(manifest, logStep)
		if logFile != nil {
			files = append(files, *logFile)
		}
	}

	return manifest, files
}

// diagnoseAppendStep records step in manifest, scrubbing its Error/Note text
// first: unlike every other field in the bundle, those strings come straight
// from Go's own error messages and can embed an absolute path (e.g. an
// os.Open failure on a home-directory db-path), so they get the same
// treatment as daemon.log's lines rather than being written raw.
func diagnoseAppendStep(manifest *diagnoseManifest, step diagnoseStepResult) {
	step.Error = diagnoseScrubLine(step.Error)
	step.Note = diagnoseScrubLine(step.Note)
	manifest.Steps = append(manifest.Steps, step)
}

func diagnoseJSONFile(name string, v any) diagnoseFile {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		// v is always one of this file's own plain structs — a marshal
		// failure here would be a programming error, not a runtime one.
		data = fmt.Appendf(nil, "{\"marshal_error\": %q}", err.Error())
	}
	return diagnoseFile{Name: name, Data: data}
}

// diagnoseHostID returns a short, non-reversible identifier for this host: a
// truncated hash of its hostname. It lets a maintainer recognize "same box,
// second report" across two bundles without ever learning the real hostname.
func diagnoseHostID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(hostname))
	return hex.EncodeToString(sum[:])[:12]
}

// diagnoseCollectDaemonInfo resolves theia.service's active state and pid via
// the same read-only systemctl probes serviceIsActive/stopService already
// use. Never starts, stops, or restarts the daemon.
func diagnoseCollectDaemonInfo(ctx context.Context) (diagnoseDaemonInfo, diagnoseStepResult) {
	active := serviceIsActive(ctx)
	info := diagnoseDaemonInfo{Active: active}
	if !active {
		return info, diagnoseStepResult{Name: "daemon", Captured: true, Note: "theia.service is not active (or systemd is unavailable on this host)"}
	}

	// #nosec G204 -- systemctlPath() only returns a fixed, hardcoded candidate
	out, err := exec.CommandContext(ctx, systemctlPath(), "show", "--property=MainPID", "--value", theiaService).Output()
	if err != nil {
		return info, diagnoseStepResult{Name: "daemon", Captured: false, Error: err.Error()}
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || pid <= 0 {
		return info, diagnoseStepResult{Name: "daemon", Captured: true, Note: "could not resolve theia.service's main pid"}
	}
	info.Pid = &pid
	return info, diagnoseStepResult{Name: "daemon", Captured: true}
}

// diagnoseReadDaemonCmdline reads pid's argv from /proc/<pid>/cmdline —
// Linux-only, and unavailable when pid is nil (daemon not running/resolved)
// or when /proc isn't present (e.g. developing on macOS). Both are reported
// as a non-fatal, non-error step: the caller falls back to --db-path/
// --log-path or theia's own CLI defaults.
//
// There is an unavoidable TOCTOU gap between diagnoseCollectDaemonInfo
// resolving pid and this function reading its cmdline: pid could have exited
// and been reused by an unrelated process in between (e.g. theia.service
// restarting mid-update). Re-checking serviceIsActive here narrows that
// window without closing it entirely — a full fix would need to pin pid to
// its start time the way procutil-style liveness checks do, which is more
// machinery than this best-effort diagnostic warrants.
func diagnoseReadDaemonCmdline(ctx context.Context, pid *int) ([]string, diagnoseStepResult) {
	if pid == nil {
		return nil, diagnoseStepResult{Name: "daemon-cmdline", Captured: false, Note: "daemon pid unavailable; falling back to flags/defaults for db-path and log-path"}
	}
	data, err := os.ReadFile(filepath.Clean(fmt.Sprintf("/proc/%d/cmdline", *pid)))
	if err != nil {
		return nil, diagnoseStepResult{Name: "daemon-cmdline", Captured: false, Error: err.Error()}
	}
	if !serviceIsActive(ctx) {
		return nil, diagnoseStepResult{Name: "daemon-cmdline", Captured: false, Note: "theia.service stopped while reading its cmdline; discarding a possibly stale/reused pid's argv"}
	}
	args := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	return args, diagnoseStepResult{Name: "daemon-cmdline", Captured: true}
}

// diagnoseArgValue returns the value passed to flag "--name" in a
// /proc/<pid>/cmdline-style argv, supporting both "--name value" and
// "--name=value" forms (cobra accepts either).
func diagnoseArgValue(args []string, name string) (string, bool) {
	prefix := "--" + name + "="
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix), true
		}
		if arg == "--"+name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// diagnoseSelectPath picks a path value in priority order: an explicit flag
// on this command, then the running daemon's own resolved argv, then
// theia's own CLI default (cmd/daemon.go) as a last-resort, unverified
// guess. Pure: no filesystem access, so the priority logic is testable on
// its own from diagnoseResolveConfig's effectful existence/readability
// checks.
func diagnoseSelectPath(flagValue string, cmdline []string, flagName, defaultValue string) (value, source string) {
	if flagValue != "" {
		return flagValue, "flag"
	}
	if v, ok := diagnoseArgValue(cmdline, flagName); ok {
		return v, "daemon-process"
	}
	return defaultValue, "default-guess"
}

// diagnoseResolveConfig selects db-path/log-path via diagnoseSelectPath,
// then probes the filesystem to report whether each resolved path actually
// exists/is readable.
func diagnoseResolveConfig(opts diagnoseOptions, cmdline []string) diagnoseConfigInfo {
	info := diagnoseConfigInfo{}
	info.DBPath, info.DBPathSource = diagnoseSelectPath(opts.DBPath, cmdline, "db-path", defaultDBPath)
	info.LogPath, info.LogPathSource = diagnoseSelectPath(opts.LogPath, cmdline, "log-path", defaultLogPath)

	if st, err := os.Stat(info.DBPath); err == nil && !st.IsDir() {
		info.DBPathExists = true
	}
	// A bare os.Open succeeds on a directory, so check IsDir explicitly —
	// otherwise a --log-path that points at a directory would be reported
	// readable even though it can never actually be tailed.
	if st, err := os.Stat(info.LogPath); err == nil && !st.IsDir() {
		if f, err := os.Open(filepath.Clean(info.LogPath)); err == nil {
			info.LogPathReadable = true
			_ = f.Close()
		}
	}

	return info
}

// diagnoseMigrationLockTimeout bounds how long diagnoseCollectDB waits for
// database.AcquireMigrationLockTimeout before giving up on the "db" step —
// see the call site's comment for why an unbounded wait isn't acceptable
// here.
const diagnoseMigrationLockTimeout = 5 * time.Second

// diagnoseCollectDB reads row counts, freshness, and migration version from
// the resolved database. It never creates the file: a missing db-path is
// itself the diagnosis for "theia stats" coming back empty, and opening it
// would silently create a fresh empty database, masking exactly that.
func diagnoseCollectDB(ctx context.Context, dbPath string, exists bool) (*diagnoseDBInfo, diagnoseStepResult) {
	if !exists {
		return nil, diagnoseStepResult{Name: "db", Captured: true, Note: fmt.Sprintf("no database file at %s — this is likely why theia stats returns empty", dbPath)}
	}

	db, err := database.Open(ctx, dbPath)
	if err != nil {
		return nil, diagnoseStepResult{Name: "db", Captured: false, Error: err.Error()}
	}
	defer database.Close(db) //nolint:errcheck // close error in a diagnostic step is not actionable

	// A bounded wait, not AcquireMigrationLock's unbounded one: diagnose's
	// whole design promises no single step blocks the bundle, so a daemon
	// (or another diagnose run) holding this lock must degrade the "db" step
	// rather than hang the command indefinitely.
	release, ok, lockErr := database.AcquireMigrationLockTimeout(dbPath, diagnoseMigrationLockTimeout)
	if lockErr != nil {
		return nil, diagnoseStepResult{Name: "db", Captured: false, Error: lockErr.Error()}
	}
	if !ok {
		return nil, diagnoseStepResult{Name: "db", Captured: false, Error: "timed out waiting for the migration lock (another theia process is migrating this database)"}
	}
	migrationErr := database.RunMigrations(db, database.MigrationsFS, database.MigrationsPath)
	_ = release() // release error is not actionable here
	if migrationErr != nil {
		return nil, diagnoseStepResult{Name: "db", Captured: false, Error: migrationErr.Error()}
	}

	version, dirty, err := database.GetCurrentVersion(db, database.MigrationsFS, database.MigrationsPath)
	if err != nil {
		return nil, diagnoseStepResult{Name: "db", Captured: false, Error: err.Error()}
	}

	info := &diagnoseDBInfo{
		Tables:           map[string]diagnoseTableInfo{},
		MigrationVersion: version,
		MigrationDirty:   dirty,
	}
	if st, err := os.Stat(dbPath); err == nil {
		info.SizeBytes = st.Size()
	}

	// Each table is collected independently: one bad/slow table must not
	// discard the rows already counted for the others, the same
	// degrade-independently principle the rest of this command follows.
	var failedTables []string
	for _, table := range []string{"hourly_stats", "hourly_status_codes", "hourly_referrers", "visitor_days"} {
		tableInfo, err := diagnoseCollectTable(ctx, db, table)
		if err != nil {
			failedTables = append(failedTables, table)
			continue
		}
		info.Tables[table] = tableInfo
	}

	step := diagnoseStepResult{Name: "db", Captured: true}
	if len(failedTables) > 0 {
		step.Note = fmt.Sprintf("could not query: %s", strings.Join(failedTables, ", "))
	}
	return info, step
}

// diagnoseCollectTable returns table's row count and, for the three
// hour-bucketed tables, the most recent bucket converted to an RFC3339
// timestamp so it reads like a normal date instead of raw (year, year_day,
// hour) columns.
func diagnoseCollectTable(ctx context.Context, db *sql.DB, table string) (diagnoseTableInfo, error) {
	var count int64
	// table is one of a fixed, hardcoded set of table names above, never
	// external input, so this is not a SQL-injection risk.
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
		return diagnoseTableInfo{}, err
	}
	info := diagnoseTableInfo{Rows: count}
	if count == 0 {
		return info, nil
	}

	if table == "visitor_days" {
		var year, yearDay int
		err := db.QueryRowContext(ctx, "SELECT year, year_day FROM visitor_days ORDER BY year DESC, year_day DESC LIMIT 1").Scan(&year, &yearDay)
		if err != nil {
			return diagnoseTableInfo{}, err
		}
		info.MostRecent = diagnoseYearDayToTime(year, yearDay, 0).Format(time.RFC3339)
		return info, nil
	}

	var year, yearDay, hour int
	// table is from the same fixed allowlist as above.
	query := fmt.Sprintf("SELECT year, year_day, hour FROM %s ORDER BY year DESC, year_day DESC, hour DESC LIMIT 1", table) //nolint:gosec // table is from a fixed internal allowlist, not external input
	if err := db.QueryRowContext(ctx, query).Scan(&year, &yearDay, &hour); err != nil {
		return diagnoseTableInfo{}, err
	}
	info.MostRecent = diagnoseYearDayToTime(year, yearDay, hour).Format(time.RFC3339)
	return info, nil
}

// diagnoseYearDayToTime converts hourly_stats' (year, year_day, hour) bucket
// columns into a UTC time.Time.
func diagnoseYearDayToTime(year, yearDay, hour int) time.Time {
	return time.Date(year, time.January, 1, hour, 0, 0, 0, time.UTC).AddDate(0, 0, yearDay-1)
}

// journalctlCandidates mirrors systemctlCandidates (cmd/update.go): fixed,
// root-owned locations, not a bare name resolved through the possibly
// attacker-influenced PATH.
var journalctlCandidates = []string{"/usr/bin/journalctl", "/bin/journalctl"}

func journalctlPath() string {
	return firstExistingCandidate(journalctlCandidates)
}

// diagnoseCollectDaemonLog shells out to journalctl for theia.service,
// time-windowed and line-capped, scrubbing every line before it's written to
// the bundle.
func diagnoseCollectDaemonLog(ctx context.Context, opts diagnoseOptions) (*diagnoseFile, diagnoseStepResult) {
	if _, err := os.Stat(journalctlPath()); err != nil {
		return nil, diagnoseStepResult{Name: "daemon-log", Captured: false, Error: "journalctl not found on this host"}
	}

	since := time.Now().Add(-opts.Since).Format("2006-01-02 15:04:05")
	args := []string{"-u", theiaService, "--no-pager", "--since", since, "-n", strconv.Itoa(opts.Lines)}
	// #nosec G204 -- journalctlPath() only returns a fixed, hardcoded candidate; args are built from opts, not external input
	out, err := exec.CommandContext(ctx, journalctlPath(), args...).Output()
	if err != nil {
		return nil, diagnoseStepResult{Name: "daemon-log", Captured: false, Error: fmt.Sprintf("running journalctl: %v", err)}
	}

	lines := diagnoseSplitLines(string(out))
	step := diagnoseStepResult{Name: "daemon-log", Captured: true}
	if len(lines) == 0 || (len(lines) == 1 && strings.TrimSpace(lines[0]) == "-- No entries --") {
		step.Note = "the systemd journal has no entries for theia.service in this window"
	}

	scrubbed := diagnoseScrubLines(lines)
	content := ""
	if len(scrubbed) > 0 {
		content = strings.Join(scrubbed, "\n") + "\n"
	}
	return &diagnoseFile{Name: "logs/daemon.log", Data: []byte(content)}, step
}

func diagnoseSplitLines(contents string) []string {
	trimmed := strings.TrimRight(contents, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// diagnoseHomePathPattern matches an absolute home-directory path (Linux/
// macOS), which would otherwise leak the OS username into a free-text log
// line. Excludes ':' from the path body: theia's own error-wrapping
// convention throughout this codebase is `fmt.Errorf("... %q: %w", path,
// err)`, and without the exclusion the greedy match swallows that trailing
// "path: rest of message" colon along with the path.
var diagnoseHomePathPattern = regexp.MustCompile(`(?:/home/[^\s"':]+|/Users/[^\s"':]+|/root(?:/[^\s"':]*)?)`)

// diagnoseSecretPattern is a best-effort scrub over free-text content
// (daemon.log lines, step Error/Note strings) for common secret-shaped
// tokens. It is a backstop, not the bundle's primary defense — daemon.log,
// config.json, and db.json only ever hold allowlisted fields to begin with.
//
// The value group stops at the first whitespace, which would leave a
// space-separated secret (e.g. `Authorization: Bearer abc123`) half
// redacted — only "Bearer" itself would match, not the token after it. The
// optional (?:bearer\s+)? consumes that scheme prefix first so the actual
// token is what ends up in the captured (and therefore redacted) group.
var diagnoseSecretPattern = regexp.MustCompile(`(?i)(authorization|api[_-]?key|secret|token|password|passwd)("?\s*[:=]\s*"?(?:bearer\s+)?)([^\s"',}]+)`)

func diagnoseScrubLine(line string) string {
	line = diagnoseHomePathPattern.ReplaceAllString(line, "<redacted-path>")
	line = diagnoseSecretPattern.ReplaceAllString(line, "$1$2[REDACTED]")
	return line
}

func diagnoseScrubLines(lines []string) []string {
	scrubbed := make([]string, len(lines))
	for i, line := range lines {
		scrubbed[i] = diagnoseScrubLine(line)
	}
	return scrubbed
}

// diagnoseWriteBundle assembles manifest.json plus every collected file into
// a tar.gz at outputPath. This is the only fatal step in the whole command:
// every collection step above degrades independently, but a bundle that
// can't be written to disk at all has nothing to report.
func diagnoseWriteBundle(outputPath string, manifest *diagnoseManifest, files []diagnoseFile) (err error) {
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	all := append([]diagnoseFile{{Name: "manifest.json", Data: manifestData}}, files...)

	f, err := os.OpenFile(filepath.Clean(outputPath), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing output file: %w", closeErr)
		}
	}()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	if writeErr := diagnoseWriteTarEntries(tw, all); writeErr != nil {
		_ = tw.Close()
		_ = gz.Close()
		_ = os.Remove(outputPath)
		return writeErr
	}
	if closeErr := tw.Close(); closeErr != nil {
		_ = gz.Close()
		_ = os.Remove(outputPath)
		return fmt.Errorf("closing tar writer: %w", closeErr)
	}
	if closeErr := gz.Close(); closeErr != nil {
		_ = os.Remove(outputPath)
		return fmt.Errorf("closing gzip writer: %w", closeErr)
	}
	return nil
}

func diagnoseWriteTarEntries(tw *tar.Writer, files []diagnoseFile) error {
	modTime := time.Now()
	for _, file := range files {
		hdr := &tar.Header{
			Name:    file.Name,
			Mode:    0640,
			Size:    int64(len(file.Data)),
			ModTime: modTime,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("writing tar header for %s: %w", file.Name, err)
		}
		if _, err := tw.Write(file.Data); err != nil {
			return fmt.Errorf("writing tar content for %s: %w", file.Name, err)
		}
	}
	return nil
}
