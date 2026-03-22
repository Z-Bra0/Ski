package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Z-Bra0/Ski/internal/app"
	"github.com/Z-Bra0/Ski/internal/source"
)

func newAddCmd(opts Options) *cobra.Command {
	var name string
	var addAll bool
	var skills []string
	var targets []string

	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Add a git skill source to the active manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}
			targetOverride := append([]string(nil), targets...)
			src, err := source.ParseGit(args[0])
			if err != nil {
				refInfo, isRef, refErr := resolveSkillReferenceInfo(svc, args[0])
				if refErr != nil {
					return refErr
				}
				if !isRef {
					return err
				}
				if len(targetOverride) == 0 {
					return fmt.Errorf("skill references can only be used with --target")
				}
				if name != "" || addAll || len(skills) > 0 {
					return fmt.Errorf("skill references cannot be combined with --name, --skill, or --all")
				}

				selected := []string(nil)
				if refInfo.UpstreamSkill != "" {
					selected = []string{refInfo.UpstreamSkill}
				}
				added, warnings, err := svc.AddSelected(refInfo.Source, selected, refInfo.Name, false, targetOverride)
				if err != nil {
					return err
				}
				printSkillWarnings(cmd, warnings)

				if len(added) == 1 {
					fmt.Fprintf(cmd.OutOrStdout(), "added %s to %s\n", added[0], manifestDisplayName(svc))
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "added %d skills to %s: %s\n", len(added), manifestDisplayName(svc), strings.Join(added, ", "))
				return nil
			}

			if len(src.Skills) > 0 && len(skills) > 0 {
				return fmt.Errorf("--skill cannot be used with legacy source selectors")
			}
			if (len(src.Skills) > 0 || len(skills) > 0) && addAll {
				return fmt.Errorf("--all cannot be used with explicit skill selectors")
			}

			selected := append([]string(nil), skills...)
			if len(selected) == 0 {
				selected = append(selected, src.Skills...)
			}
			added, warnings, err := svc.AddSelected(args[0], selected, name, addAll, targetOverride)
			if err != nil {
				var multiErr app.MultiSkillSelectionError
				if !errors.As(err, &multiErr) {
					return err
				}

				selected, err = resolveAddSelection(cmd, opts, args[0], selected, multiErr.Skills, addAll)
				if err != nil {
					return err
				}
				added, warnings, err = svc.AddSelected(args[0], selected, name, addAll, targetOverride)
				if err != nil {
					return err
				}
			}
			printSkillWarnings(cmd, warnings)

			if len(added) == 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "added %s to %s\n", added[0], manifestDisplayName(svc))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %d skills to %s: %s\n", len(added), manifestDisplayName(svc), strings.Join(added, ", "))
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Override the local skill name for one added skill only; cannot be used when adding multiple skills")
	cmd.Flags().StringSliceVar(&skills, "skill", nil, "Select one or more discovered upstream skills by name")
	cmd.Flags().BoolVar(&addAll, "all", false, "Add all skills discovered in the repository")
	cmd.Flags().StringSliceVar(&targets, "target", nil, "Override targets for the added skill entries (for example claude or dir:./agent-skills/claude)")
	return cmd
}

func resolveAddSelection(cmd *cobra.Command, opts Options, rawSource string, explicit, discovered []string, addAll bool) ([]string, error) {
	if len(explicit) > 0 {
		return explicit, nil
	}

	if addAll {
		return discovered, nil
	}

	if !opts.IsTTY() {
		return nil, fmt.Errorf("multiple skills found; rerun with %s %s or --all", strings.TrimSpace(rawSource), formatSkillFlags(discovered))
	}

	return promptForSkills(cmd, opts, discovered)
}

func formatSkillFlags(skills []string) string {
	parts := make([]string, 0, len(skills))
	for _, skill := range skills {
		parts = append(parts, "--skill "+skill)
	}
	return strings.Join(parts, " ")
}

func promptForSkills(cmd *cobra.Command, opts Options, discovered []string) ([]string, error) {
	promptOpts := opts
	promptOpts.Stdin = cmd.InOrStdin()
	promptOpts.Stdout = cmd.OutOrStdout()

	return runMultiSelectPrompt(promptOpts, MultiSelectRequest{
		Title:       "Select skills to add",
		Description: "Use arrows to move, space to toggle, and enter to confirm.",
		Options:     discovered,
		MinSelected: 1,
	})
}
