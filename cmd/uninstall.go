package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"codeberg.org/Elysium_Labs/theia/internal/ui"
	"github.com/spf13/cobra"
)

// theiaDataDir must stay in sync with DATA_DIR in install.sh.
const theiaDataDir = "/var/lib/theia"

const theiaServiceUnit = "/etc/systemd/system/theia.service"

// stopAndDisableService stops and disables theia.service before the binary
// it points at is removed, and deletes the unit file install.sh wrote.
// Best-effort: a non-systemd environment (dev machine, container, tests)
// just skips this silently.
func stopAndDisableService(ctx context.Context, out io.Writer) {
	if !serviceIsActive(ctx) {
		if _, err := os.Stat(theiaServiceUnit); err != nil {
			return
		}
	} else if err := exec.CommandContext(ctx, "systemctl", "stop", theiaService).Run(); err != nil {
		_, _ = fmt.Fprintf(out, "%s could not stop %s: %v\n", ui.LabelWarning.Render("warning"), theiaService, err)
	}

	_ = exec.CommandContext(ctx, "systemctl", "disable", theiaService).Run()

	if err := os.Remove(theiaServiceUnit); err != nil && !os.IsNotExist(err) {
		_, _ = fmt.Fprintf(out, "%s could not remove %s: %v\n", ui.LabelWarning.Render("warning"), theiaServiceUnit, err)
		return
	}
	_, _ = fmt.Fprintf(out, "%s removed %s\n", ui.LabelSuccess.Render("✓"), theiaServiceUnit)
}

// runUninstall implements `theia system uninstall` against explicit paths so it
// can be exercised in tests without touching the real installed binary,
// systemd, or os.Executable() (which, under `go test`, is the test binary
// itself).
func runUninstall(ctx context.Context, in io.Reader, out io.Writer, exePath, dataDir string, yes, purge bool) error {
	if !yes && !ui.Confirm(in, out, fmt.Sprintf("Remove theia (%s)?", exePath), false) {
		_, _ = fmt.Fprintln(out, "Canceled.")
		return nil
	}

	stopAndDisableService(ctx, out)

	if err := os.Remove(exePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", exePath, err)
	}
	_, _ = fmt.Fprintf(out, "%s removed %s\n", ui.LabelSuccess.Render("✓"), exePath)

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return nil
	}

	removeData := purge
	if !removeData && !yes {
		removeData = ui.Confirm(in, out, fmt.Sprintf("Also remove theia data (%s)?", dataDir), false)
	}

	if removeData {
		if err := os.RemoveAll(dataDir); err != nil {
			return fmt.Errorf("removing %s: %w", dataDir, err)
		}
		_, _ = fmt.Fprintf(out, "%s removed %s\n", ui.LabelSuccess.Render("✓"), dataDir)
	} else {
		_, _ = fmt.Fprintf(out, "%s data left in place — remove manually: %s\n",
			ui.TextMuted.Render("i"), ui.TextCommand.Render("rm -rf "+dataDir))
	}
	return nil
}

func newUninstallCmd() *cobra.Command {
	var yes bool
	var purge bool

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the theia binary and systemd service",
		Long: `Remove the theia binary and its systemd service.

By default the data directory (` + theiaDataDir + `, holding theia.db) is left
in place and a manual cleanup hint is printed. Pass --purge to remove it too.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exePath, err := currentBinaryPath()
			if err != nil {
				return err
			}
			return runUninstall(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), exePath, theiaDataDir, yes, purge)
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the binary-removal confirmation prompt")
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove data (theia.db) without prompting")
	return cmd
}
