package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Elysium-Labs-EU/theia/database"
)

func TestDiagnoseArgValue(t *testing.T) {
	tests := []struct {
		name   string
		flag   string
		want   string
		args   []string
		wantOK bool
	}{
		{name: "space form", flag: "db-path", want: "/var/lib/theia/theia.db", args: []string{"theia", "daemon", "--db-path", "/var/lib/theia/theia.db"}, wantOK: true},
		{name: "equals form", flag: "db-path", want: "/var/lib/theia/theia.db", args: []string{"theia", "daemon", "--db-path=/var/lib/theia/theia.db"}, wantOK: true},
		{name: "missing", flag: "db-path", args: []string{"theia", "daemon", "--log-path", "/x"}},
		{name: "trailing flag with no value", flag: "db-path", args: []string{"theia", "daemon", "--db-path"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := diagnoseArgValue(tt.args, tt.flag)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("diagnoseArgValue(%v, %q) = (%q, %v), want (%q, %v)", tt.args, tt.flag, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestDiagnoseResolveConfig(t *testing.T) {
	cmdline := []string{"theia", "daemon", "--db-path", "/from/daemon.db", "--log-path=/from/daemon.log"}

	t.Run("flag wins over daemon process", func(t *testing.T) {
		info := diagnoseResolveConfig(diagnoseOptions{DBPath: "/explicit.db"}, cmdline)
		if info.DBPath != "/explicit.db" || info.DBPathSource != "flag" {
			t.Errorf("got %+v", info)
		}
	})

	t.Run("daemon process wins over default", func(t *testing.T) {
		info := diagnoseResolveConfig(diagnoseOptions{}, cmdline)
		if info.DBPath != "/from/daemon.db" || info.DBPathSource != "daemon-process" {
			t.Errorf("db: got %+v", info)
		}
		if info.LogPath != "/from/daemon.log" || info.LogPathSource != "daemon-process" {
			t.Errorf("log: got %+v", info)
		}
	})

	t.Run("falls back to default guess with no cmdline", func(t *testing.T) {
		info := diagnoseResolveConfig(diagnoseOptions{}, nil)
		if info.DBPath != "./theia.db" || info.DBPathSource != "default-guess" {
			t.Errorf("got %+v", info)
		}
		if info.LogPath != "/var/log/nginx/access.log" || info.LogPathSource != "default-guess" {
			t.Errorf("got %+v", info)
		}
	})
}

func TestDiagnoseYearDayToTime(t *testing.T) {
	got := diagnoseYearDayToTime(2026, 1, 14)
	want := time.Date(2026, time.January, 1, 14, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDiagnoseCollectDB_MissingFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.db")

	info, step := diagnoseCollectDB(t.Context(), dbPath, false)
	if info != nil {
		t.Errorf("expected nil info for a missing db file, got %+v", info)
	}
	if !step.Captured {
		t.Errorf("a missing db file should still be a captured (non-error) step, got %+v", step)
	}
	if step.Note == "" {
		t.Errorf("expected a note explaining the missing file")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Errorf("diagnoseCollectDB must not create the db file when it doesn't exist")
	}
}

func TestDiagnoseCollectDB_RowCountsAndFreshness(t *testing.T) {
	db, dbPath := setupCmdTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	now := time.Date(2026, time.March, 5, 10, 0, 0, 0, time.UTC)
	insertStat(t, db, "/", "example.com", now, statSeed{PageViews: 5, UniqueVisitors: 2, BotViews: 1})

	info, step := diagnoseCollectDB(t.Context(), dbPath, true)
	if !step.Captured {
		t.Fatalf("expected captured step, got %+v", step)
	}
	if info == nil {
		t.Fatalf("expected non-nil db info")
	}
	if info.Tables["hourly_stats"].Rows != 1 {
		t.Errorf("hourly_stats rows: got %d, want 1", info.Tables["hourly_stats"].Rows)
	}
	if info.Tables["hourly_stats"].MostRecent != now.Format(time.RFC3339) {
		t.Errorf("hourly_stats most_recent: got %q, want %q", info.Tables["hourly_stats"].MostRecent, now.Format(time.RFC3339))
	}
	if info.Tables["visitor_days"].Rows != 2 {
		t.Errorf("visitor_days rows: got %d, want 2", info.Tables["visitor_days"].Rows)
	}
	if info.Tables["hourly_referrers"].Rows != 0 {
		t.Errorf("hourly_referrers rows: got %d, want 0", info.Tables["hourly_referrers"].Rows)
	}
}

func TestDiagnoseCollectDB_PartialTableFailureKeepsOthers(t *testing.T) {
	db, dbPath := setupCmdTestDB(t)
	defer database.Close(db) //nolint:errcheck // close error in defer is not actionable

	now := time.Now()
	insertStat(t, db, "/", "example.com", now, statSeed{PageViews: 5, UniqueVisitors: 2, BotViews: 0})

	if _, err := db.ExecContext(t.Context(), "DROP TABLE hourly_referrers"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	info, step := diagnoseCollectDB(t.Context(), dbPath, true)
	if !step.Captured {
		t.Fatalf("one failing table must not fail the whole db step, got %+v", step)
	}
	if step.Note == "" {
		t.Errorf("expected a note listing the failed table")
	}
	if info == nil {
		t.Fatalf("expected non-nil db info despite one failing table")
	}
	if info.Tables["hourly_stats"].Rows != 1 {
		t.Errorf("hourly_stats should still be collected, got %+v", info.Tables["hourly_stats"])
	}
	if _, ok := info.Tables["hourly_referrers"]; ok {
		t.Errorf("hourly_referrers should be absent (its query failed), got %+v", info.Tables["hourly_referrers"])
	}
}

func TestDiagnoseScrubLine(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"opening log file /Users/alice/access.log", "opening log file <redacted-path>"},
		{`token: "abc123"`, `token: "[REDACTED]"`},
		{"Authorization: Bearer abc123XYZsecrettoken", "Authorization: Bearer [REDACTED]"},
		{"nothing sensitive here", "nothing sensitive here"},
	}
	for _, tt := range tests {
		if got := diagnoseScrubLine(tt.in); got != tt.want {
			t.Errorf("diagnoseScrubLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDiagnoseAppendStep_ScrubsErrorAndNote(t *testing.T) {
	manifest := &diagnoseManifest{}
	diagnoseAppendStep(manifest, diagnoseStepResult{
		Name:  "db",
		Error: "opening /Users/alice/theia.db: permission denied",
		Note:  "token: \"abc123\"",
	})

	got := manifest.Steps[0]
	if got.Error != "opening <redacted-path>: permission denied" {
		t.Errorf("Error not scrubbed: %q", got.Error)
	}
	if got.Note != `token: "[REDACTED]"` {
		t.Errorf("Note not scrubbed: %q", got.Note)
	}
}

func TestDiagnoseResolveConfig_LogPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	info := diagnoseResolveConfig(diagnoseOptions{LogPath: dir}, nil)
	if info.LogPathReadable {
		t.Errorf("a directory log-path must not be reported readable, got %+v", info)
	}
}

func TestDiagnoseWriteBundle(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	manifest := &diagnoseManifest{Steps: []diagnoseStepResult{{Name: "version", Captured: true}}}
	files := []diagnoseFile{{Name: "version.json", Data: []byte(`"dev"`)}}

	if err := diagnoseWriteBundle(outputPath, manifest, files); err != nil {
		t.Fatalf("diagnoseWriteBundle: %v", err)
	}

	f, err := os.Open(filepath.Clean(outputPath))
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer f.Close() //nolint:errcheck // close error in defer is not actionable

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gz.Close() //nolint:errcheck // close error in defer is not actionable

	tr := tar.NewReader(gz)
	names := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("tar content read: %v", err)
		}
		names[hdr.Name] = data
	}

	if _, ok := names["manifest.json"]; !ok {
		t.Errorf("bundle missing manifest.json, got entries: %v", names)
	}
	if _, ok := names["version.json"]; !ok {
		t.Errorf("bundle missing version.json, got entries: %v", names)
	}

	var got diagnoseManifest
	if err := json.Unmarshal(names["manifest.json"], &got); err != nil {
		t.Fatalf("unmarshal manifest.json: %v", err)
	}
	if len(got.Steps) != 1 || got.Steps[0].Name != "version" {
		t.Errorf("manifest.json round-trip mismatch: %+v", got)
	}
}

func TestDiagnoseCollect_NeverFatal(t *testing.T) {
	// No running daemon, no db file, no journalctl expected on a dev/CI
	// host — diagnoseCollect must still return a full manifest rather than
	// panicking or erroring out.
	manifest, files := diagnoseCollect(t.Context(), diagnoseOptions{
		DBPath:  filepath.Join(t.TempDir(), "missing.db"),
		LogPath: filepath.Join(t.TempDir(), "missing.log"),
		Since:   time.Minute,
		Lines:   10,
	})

	if manifest == nil {
		t.Fatal("expected non-nil manifest")
	}
	stepNames := map[string]bool{}
	for _, s := range manifest.Steps {
		stepNames[s.Name] = true
	}
	for _, want := range []string{"version", "daemon", "daemon-cmdline", "config", "db"} {
		if !stepNames[want] {
			t.Errorf("manifest missing step %q, got %+v", want, manifest.Steps)
		}
	}

	fileNames := map[string]bool{}
	for _, f := range files {
		fileNames[f.Name] = true
	}
	if !fileNames["config.json"] {
		t.Errorf("expected config.json among collected files, got %v", fileNames)
	}
}
