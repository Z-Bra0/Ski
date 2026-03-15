package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"ski/internal/app"
)

func newListCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List skills declared in the current project",
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
			infos, err := svc.List()
			if err != nil {
				return err
			}

			if len(infos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no skills installed")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSOURCE\tCOMMIT\tTARGETS")
			for _, info := range infos {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					info.Name,
					info.Source,
					info.Commit,
					strings.Join(info.Targets, ","),
				)
			}
			return w.Flush()
		},
	}
}
