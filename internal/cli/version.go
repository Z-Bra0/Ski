package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Z-Bra0/Ski/internal/buildinfo"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the ski version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Version)
		},
	}
}
