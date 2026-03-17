package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"ski/internal/app"
	"ski/internal/manifest"
	"ski/internal/target"
)

func newInitCmd(opts Options) *cobra.Command {
	var targets []string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a ski manifest in the active scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(cmd, opts)
			if err != nil {
				return err
			}

			selectedTargets := append([]string(nil), targets...)
			if len(selectedTargets) == 0 && opts.IsTTY() {
				selectedTargets, err = promptForInitTargets(cmd, opts, svc)
				if err != nil {
					return err
				}
			} else {
				selectedTargets, err = normalizeInitTargets(selectedTargets, svc, false)
				if err != nil {
					return err
				}
			}

			path, err := svc.Init()
			if err != nil {
				return err
			}
			if len(selectedTargets) > 0 {
				doc, err := manifest.ReadFile(path)
				if err != nil {
					return err
				}
				doc.Targets = selectedTargets
				if err := manifest.WriteFile(path, *doc); err != nil {
					return err
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&targets, "target", nil, "Set one or more initial targets (for example claude or dir:./agent-skills/claude)")
	return cmd
}

func promptForInitTargets(cmd *cobra.Command, opts Options, svc app.Service) ([]string, error) {
	promptOpts := opts
	promptOpts.Stdin = cmd.InOrStdin()
	promptOpts.Stdout = cmd.OutOrStdout()

	builtins, err := runMultiSelectPrompt(promptOpts, MultiSelectRequest{
		Title:       "Select targets",
		Description: "Choose built-in targets to initialize now. Leave empty to configure them later.",
		Options:     []string{"claude", "codex", "cursor", "openclaw"},
	})
	if err != nil {
		return nil, err
	}

	customDescription := "Optional. Enter additional target directories as comma-separated paths relative to the project root."
	placeholder := "./agent-skills/claude"
	if svc.Global {
		customDescription = "Optional. Enter additional target directories as comma-separated paths relative to your home directory. You may also use ~/..."
		placeholder = "agent-skills/claude"
	}

	customInput, err := runTextPrompt(promptOpts, TextPromptRequest{
		Title:       "Custom target directories",
		Description: customDescription,
		Placeholder: placeholder,
	})
	if err != nil {
		return nil, err
	}

	customTargets, err := normalizeInitTargets(splitCommaList(customInput), svc, true)
	if err != nil {
		return nil, err
	}
	return dedupeStrings(append(builtins, customTargets...)), nil
}

func normalizeInitTargets(values []string, svc app.Service, allowBareCustom bool) ([]string, error) {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		targetName := strings.TrimSpace(value)
		if targetName == "" {
			continue
		}
		if allowBareCustom && !isBuiltInTarget(targetName) && !strings.HasPrefix(targetName, "dir:") {
			targetName = "dir:" + targetName
		}

		var err error
		if svc.Global {
			_, err = target.GlobalSkillDir(svc.HomeDir, targetName)
		} else {
			_, err = target.SkillDir(svc.ProjectDir, targetName)
		}
		if err != nil {
			return nil, err
		}
		if _, ok := seen[targetName]; ok {
			continue
		}
		seen[targetName] = struct{}{}
		normalized = append(normalized, targetName)
	}
	return normalized, nil
}

func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func dedupeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func isBuiltInTarget(value string) bool {
	switch value {
	case "claude", "codex", "cursor", "openclaw":
		return true
	default:
		return false
	}
}
