package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newListCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List skills declared in the active scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}
			infos, err := svc.List()
			if err != nil {
				return err
			}

			if len(infos) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no skills installed")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "#\tNAME\tSTATUS\tSOURCE\tUPSTREAM\tCOMMIT\tTARGETS")
			for i, info := range infos {
				status := "enabled"
				if !info.Enabled {
					status = "disabled"
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					i+1,
					info.Name,
					status,
					info.Source,
					info.UpstreamSkill,
					info.Commit,
					strings.Join(info.Targets, ","),
				)
			}
			return w.Flush()
		},
	}
}
