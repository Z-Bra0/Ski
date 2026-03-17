package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInitCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a ski manifest in the active scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}
			path, err := svc.Init()
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)
			return nil
		},
	}
}
