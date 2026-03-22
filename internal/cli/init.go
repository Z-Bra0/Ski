package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Z-Bra0/Ski/internal/app"
	"github.com/Z-Bra0/Ski/internal/target"
)

var initBuiltInTargets = target.BuiltInNames()

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
			if err := svc.CheckInitAvailable(); err != nil {
				return err
			}

			selectedTargets := append([]string(nil), targets...)
			if len(selectedTargets) == 0 && opts.IsTTY() {
				selectedTargets, err = promptForInitTargets(cmd, opts)
				if err != nil {
					return err
				}
			} else {
				selectedTargets, err = normalizeInitTargets(selectedTargets, svc, false)
				if err != nil {
					return err
				}
			}

			path, err := svc.InitWithTargets(selectedTargets)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&targets, "target", nil, "Set one or more initial targets (for example claude or dir:./agent-skills/claude)")
	return cmd
}

func promptForInitTargets(cmd *cobra.Command, opts Options) ([]string, error) {
	promptOpts := opts
	promptOpts.Stdin = cmd.InOrStdin()
	promptOpts.Stdout = cmd.OutOrStdout()

	builtins, err := runMultiSelectPrompt(promptOpts, MultiSelectRequest{
		Title:       "Select targets",
		Description: "Choose built-in targets to initialize now. Use space to toggle multiple items and enter to confirm. Leave empty to configure them later.",
		Options:     append([]string{"all"}, initBuiltInTargets...),
		Height:      10,
	})
	if err != nil {
		return nil, err
	}
	return dedupeStrings(expandInitBuiltInSelections(builtins)), nil
}

func normalizeInitTargets(values []string, svc app.Service, allowBareCustom bool) ([]string, error) {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	seenDirs := make(map[string]string, len(values))
	for _, value := range values {
		targetName := strings.TrimSpace(value)
		if targetName == "" {
			continue
		}
		if allowBareCustom && !isBuiltInTarget(targetName) && !strings.HasPrefix(targetName, "dir:") {
			targetName = "dir:" + targetName
		}

		var err error
		resolvedDir := ""
		if svc.Global {
			resolvedDir, err = target.GlobalSkillDir(svc.HomeDir, targetName)
		} else {
			resolvedDir, err = target.SkillDir(svc.ProjectDir, targetName)
		}
		if err != nil {
			return nil, err
		}
		if previous, ok := seenDirs[resolvedDir]; ok && previous != targetName {
			return nil, fmt.Errorf("targets %q and %q resolve to the same directory %s", previous, targetName, resolvedDir)
		}
		seenDirs[resolvedDir] = targetName
		if _, ok := seen[targetName]; ok {
			continue
		}
		seen[targetName] = struct{}{}
		normalized = append(normalized, targetName)
	}
	return normalized, nil
}

func expandInitBuiltInSelections(values []string) []string {
	expanded := make([]string, 0, len(values)+len(initBuiltInTargets))
	includeAll := false
	for _, value := range values {
		if strings.TrimSpace(value) == "all" {
			includeAll = true
			continue
		}
		expanded = append(expanded, value)
	}
	if !includeAll {
		return expanded
	}
	return append(append([]string(nil), initBuiltInTargets...), expanded...)
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
	for _, targetName := range initBuiltInTargets {
		if value == targetName {
			return true
		}
	}
	return false
}
