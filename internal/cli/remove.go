package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"ski/internal/app"
)

func newRemoveCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <skill>",
		Short: "Remove a skill from the project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := opts.Getwd()
			if err != nil {
				return fmt.Errorf("resolve working directory: %w", err)
			}
			homeDir, err := opts.GetHomeDir()
			if err != nil {
				return fmt.Errorf("resolve home directory: %w", err)
			}

			name := args[0]
			svc := app.Service{ProjectDir: cwd, HomeDir: homeDir}
			if err := svc.Remove(name); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "removed skill %q\n", name)
			return nil
		},
	}
}
