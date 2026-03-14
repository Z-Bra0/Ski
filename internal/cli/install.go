package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"ski/internal/app"
)

func newInstallCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install skills from ski.toml and ski.lock.json",
		Args:  cobra.NoArgs,
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
			count, err := svc.Install()
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "installed %d skills\n", count)
			return nil
		},
	}
}
