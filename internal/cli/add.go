package cli

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"ski/internal/app"
	"ski/internal/source"
)

func newAddCmd(opts Options) *cobra.Command {
	var name string
	var addAll bool
	var skills []string

	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Add a git skill source to the active manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}
			src, err := source.ParseGit(args[0])
			if err != nil {
				return err
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
			added, err := svc.AddSelected(args[0], selected, name)
			if err != nil {
				var multiErr app.MultiSkillSelectionError
				if !errors.As(err, &multiErr) {
					return err
				}

				selected, err = resolveAddSelection(cmd, opts, args[0], selected, multiErr.Skills, addAll)
				if err != nil {
					return err
				}
				added, err = svc.AddSelected(args[0], selected, name)
				if err != nil {
					return err
				}
			}

			if len(added) == 1 {
				fmt.Fprintf(cmd.OutOrStdout(), "added %s to %s\n", added[0], manifestDisplayName(svc))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %d skills to %s: %s\n", len(added), manifestDisplayName(svc), strings.Join(added, ", "))
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Override the local skill name written to ski.toml for a single selected skill")
	cmd.Flags().StringSliceVar(&skills, "skill", nil, "Select one or more discovered upstream skills by name")
	cmd.Flags().BoolVar(&addAll, "all", false, "Add all skills discovered in the repository")
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

	return promptForSkills(cmd, discovered)
}

func formatSkillFlags(skills []string) string {
	parts := make([]string, 0, len(skills))
	for _, skill := range skills {
		parts = append(parts, "--skill "+skill)
	}
	return strings.Join(parts, " ")
}

func promptForSkills(cmd *cobra.Command, discovered []string) ([]string, error) {
	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	for {
		fmt.Fprintln(out, "multiple skills found:")
		for i, name := range discovered {
			fmt.Fprintf(out, "  %d. %s\n", i+1, name)
		}
		fmt.Fprint(out, "select skill numbers or names (comma-separated), or 'all': ")

		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return nil, fmt.Errorf("read selection: %w", err)
		}

		selected, parseErr := parsePromptSelection(strings.TrimSpace(line), discovered)
		if parseErr == nil {
			return selected, nil
		}
		fmt.Fprintf(out, "invalid selection: %v\n", parseErr)
	}
}

func parsePromptSelection(input string, discovered []string) ([]string, error) {
	if input == "" {
		return nil, fmt.Errorf("selection is required")
	}
	if strings.EqualFold(input, "all") {
		return append([]string(nil), discovered...), nil
	}

	byIndex := make(map[string]string, len(discovered))
	byName := make(map[string]string, len(discovered))
	for i, name := range discovered {
		byIndex[fmt.Sprintf("%d", i+1)] = name
		byName[name] = name
	}

	seen := make(map[string]struct{}, len(discovered))
	selected := make([]string, 0, len(discovered))
	for _, part := range strings.Split(input, ",") {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, fmt.Errorf("empty selection")
		}
		name, ok := byIndex[token]
		if !ok {
			name, ok = byName[token]
		}
		if !ok {
			return nil, fmt.Errorf("unknown skill %q", token)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate selection %q", name)
		}
		seen[name] = struct{}{}
		selected = append(selected, name)
	}

	return selected, nil
}
