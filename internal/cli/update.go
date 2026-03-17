package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUpdateCmd(opts Options) *cobra.Command {
	var check bool

	cmd := &cobra.Command{
		Use:   "update [skill]",
		Short: "Update skills in the active scope to newer upstream revisions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}

			name := ""
			if len(args) == 1 {
				name = args[0]
			}

			if check {
				updates, err := svc.CheckUpdates(name)
				if err != nil {
					return err
				}
				if len(updates) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "all skills up to date")
					return nil
				}
				for _, update := range updates {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s -> %s\n",
						update.Name,
						shortCommit(update.CurrentCommit),
						shortCommit(update.LatestCommit),
					)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d skills can be updated\n", len(updates))
				return nil
			}

			updates, err := svc.Update(name)
			if err != nil {
				return err
			}
			if len(updates) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "all skills up to date")
				return nil
			}
			for _, update := range updates {
				fmt.Fprintf(cmd.OutOrStdout(), "updated %s %s -> %s\n",
					update.Name,
					shortCommit(update.CurrentCommit),
					shortCommit(update.LatestCommit),
				)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated %d skills\n", len(updates))
			return nil
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "Report available updates without changing anything")
	return cmd
}

func shortCommit(commit string) string {
	if commit == "" {
		return "(none)"
	}
	if len(commit) < 7 {
		return commit
	}
	return commit[:7]
}
