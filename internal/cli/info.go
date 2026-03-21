package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Z-Bra0/Ski/internal/app"
)

func newInfoCmd(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <skill>",
		Short: "Show detailed state for one declared skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}

			name, _, err := resolveSkillReferenceName(svc, args[0])
			if err != nil {
				return err
			}
			if name == "" {
				name = args[0]
			}

			info, err := svc.Info(name)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "name: %s\n", info.Name)
			fmt.Fprintf(out, "source: %s\n", info.Source)
			fmt.Fprintf(out, "upstream: %s\n", info.UpstreamSkill)
			if info.Version != "" {
				fmt.Fprintf(out, "version: %s\n", info.Version)
			}
			if info.Commit != "" {
				fmt.Fprintf(out, "commit: %s\n", info.Commit)
			}
			if info.Integrity != "" {
				fmt.Fprintf(out, "integrity: %s\n", info.Integrity)
			}
			if info.StorePath != "" {
				fmt.Fprintf(out, "store path: %s\n", info.StorePath)
			} else if info.StoreError != "" {
				fmt.Fprintf(out, "store error: %s\n", info.StoreError)
			}
			fmt.Fprintf(out, "targets: %s\n", strings.Join(targetNames(info.Targets), ", "))
			for _, target := range info.Targets {
				fmt.Fprintf(out, "target %s: %s (%s)", target.Name, target.Status, target.Path)
				if target.CurrentPath != "" && (info.StorePath == "" || target.CurrentPath != info.StorePath) {
					fmt.Fprintf(out, " -> %s", target.CurrentPath)
				}
				fmt.Fprintln(out)
			}
			return nil
		},
	}
}

func targetNames(targets []app.TargetLinkInfo) []string {
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.Name)
	}
	return names
}
