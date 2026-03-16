package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"ski/internal/app"
)

func newDoctorCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check for broken links and manifest/lockfile inconsistencies",
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
			findings, err := svc.Doctor()
			if err != nil {
				return err
			}

			if len(findings) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "doctor: ok")
				return nil
			}

			for _, finding := range findings {
				fmt.Fprintln(cmd.OutOrStdout(), finding.String())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "doctor found %d issues\n", len(findings))
			return fmt.Errorf("doctor found %d issues", len(findings))
		},
	}
}
