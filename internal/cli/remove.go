package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newRemoveCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <skill>",
		Short: "Remove a skill from the active scope",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}

			name := args[0]
			if err := svc.Remove(name); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "removed skill %q\n", name)
			return nil
		},
	}
}
