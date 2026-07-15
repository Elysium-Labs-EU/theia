package cmd

import (
	"codeberg.org/Elysium_Labs/theia/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the theia version",
		Long:  `Print the current theia version, git commit hash, and build date.`,
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Println(buildinfo.Get())
		},
	}
}

// newSystemCmd builds the `theia system` command group fresh each call —
// cobra commands can only have one parent, and newRootCmd() may be
// constructed more than once per process (e.g. once per test), so reusing
// singleton subcommand vars here would let a later rootCmd steal them from
// an earlier one.
func newSystemCmd() *cobra.Command {
	systemCmd := &cobra.Command{
		Use:   "system",
		Short: "Manage the theia binary and check its version",
		Long:  `Manage the theia binary and runtime: check for updates, remove it, or print its version.`,
	}

	systemCmd.AddCommand(newVersionCmd())
	systemCmd.AddCommand(newUpdateCmd())
	systemCmd.AddCommand(newUninstallCmd())

	return systemCmd
}
