package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDoctorCmd(opts Options) *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
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
			if !fix {
				for _, finding := range findings {
					fmt.Fprintln(cmd.OutOrStdout(), finding.String())
				}
				fmt.Fprintf(cmd.OutOrStdout(), "doctor found %d issues\n", len(findings))
				return fmt.Errorf("doctor found %d issues", len(findings))
			}

			results, err := svc.Fix(findings)
			if err != nil {
				return err
			}

			fixedCount := 0
			for _, result := range results {
				fmt.Fprintln(cmd.OutOrStdout(), result.Finding.String())
				switch {
				case result.Fixed:
					fixedCount++
					fmt.Fprintf(cmd.OutOrStdout(), "  fixed: %s\n", result.Note)
				case result.Err != nil:
					fmt.Fprintf(cmd.OutOrStdout(), "  error: %v\n", result.Err)
				case result.Note != "":
					fmt.Fprintf(cmd.OutOrStdout(), "  skipped: %s\n", result.Note)
				}
			}

			remaining, err := svc.Doctor()
			if err != nil {
				return err
			}
			if len(remaining) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "doctor: fixed %d issues\n", fixedCount)
				return nil
			}

			for _, finding := range remaining {
				fmt.Fprintln(cmd.OutOrStdout(), finding.String())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "doctor: fixed %d issues, %d require manual intervention\n", fixedCount, len(remaining))
			return fmt.Errorf("doctor found %d issues", len(remaining))
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Repair fixable issues in the active scope")
	return cmd
}
