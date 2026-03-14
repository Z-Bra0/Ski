package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"ski/internal/app"
)

func newAddCmd(opts Options) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Add a git skill source to ski.toml",
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

			svc := app.Service{ProjectDir: cwd, HomeDir: homeDir}
			skillName, err := svc.Add(args[0], name)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "added %s to ski.toml\n", skillName)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Override the default skill name derived from the git URL")
	return cmd
}
