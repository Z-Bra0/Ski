package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newRemoveCmd(opts Options) *cobra.Command {
	var targets []string

	cmd := &cobra.Command{
		Use:   "remove <skill>",
		Short: "Remove a skill from the active scope",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}
			targetOverride := append([]string(nil), targets...)

			name, _, err := resolveSkillReferenceName(svc, args[0])
			if err != nil {
				return err
			}
			if name == "" {
				name = args[0]
			}
			if err := svc.Remove(name, targetOverride); err != nil {
				return err
			}

			if len(targetOverride) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "removed skill %q from targets: %s\n", name, strings.Join(targetOverride, ", "))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed skill %q\n", name)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&targets, "target", nil, "Remove the skill only from the specified targets (for example claude or dir:./agent-skills/claude)")
	return cmd
}
