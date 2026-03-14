package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"ski/internal/app"
)

func newInitCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a ski.toml manifest in the current project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := opts.Getwd()
			if err != nil {
				return fmt.Errorf("resolve working directory: %w", err)
			}

			svc := app.Service{ProjectDir: cwd}
			path, err := svc.Init()
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)
			return nil
		},
	}
}
