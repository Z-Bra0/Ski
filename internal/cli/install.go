package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInstallCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install skills from the active manifest and lockfile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}
			count, err := svc.Install()
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "installed %d skills\n", count)
			return nil
		},
	}
}
