package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check for target drift and inconsistencies in the active scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}
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
