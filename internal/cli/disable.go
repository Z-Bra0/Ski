package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDisableCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "disable <skill>",
		Short: "Disable a declared skill without removing it from the manifest",
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

			if err := svc.Disable(name); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "disabled skill %q\n", name)
			return nil
		},
	}
}
