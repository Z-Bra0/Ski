package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var check bool

	cmd := &cobra.Command{
		Use:   "update [skill]",
		Short: "Update skills to newer upstream revisions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if check {
				return fmt.Errorf("not implemented: ski update --check")
			}
			return fmt.Errorf("not implemented: ski update")
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "Report available updates without changing anything")
	return cmd
}
