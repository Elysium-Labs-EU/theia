package cmd

import (
	"github.com/spf13/cobra"

	"theia/internal/buildinfo"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the theia version",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println(buildinfo.Get())
		},
	}
}
