package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newEnableCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "enable <skill>",
		Short: "Enable a declared disabled skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}

			name, _, err := resolveSkillReferenceName(svc, args[0])
			if err != nil {
				return err
			}
			if name == "" {
				name = args[0]
			}

			if err := svc.Enable(name); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "enabled skill %q\n", name)
			return nil
		},
	}
}
