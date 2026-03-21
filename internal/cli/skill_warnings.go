package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Z-Bra0/Ski/internal/skill"
)

type skillWarningGroup struct {
	name     string
	path     string
	messages []string
}

func printSkillWarnings(cmd *cobra.Command, warnings []skill.ValidationWarning) {
	if len(warnings) == 0 {
		return
	}

	groups, count := groupSkillWarnings(warnings)
	fmt.Fprintf(
		cmd.ErrOrStderr(),
		"warning: strict Agent Skills mismatches found in %d %s (%d %s)\n",
		len(groups),
		pluralize(len(groups), "skill", "skills"),
		count,
		pluralize(count, "warning", "warnings"),
	)
	for i, group := range groups {
		if i > 0 {
			fmt.Fprintln(cmd.ErrOrStderr())
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "skill %q (%s)\n", group.name, group.path)
		for _, message := range group.messages {
			fmt.Fprintf(cmd.ErrOrStderr(), "- %s\n", message)
		}
	}
}

func groupSkillWarnings(warnings []skill.ValidationWarning) ([]skillWarningGroup, int) {
	sorted := append([]skill.ValidationWarning(nil), warnings...)
	slices.SortFunc(sorted, func(a, b skill.ValidationWarning) int {
		if cmp := strings.Compare(a.Name, b.Name); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Path, b.Path); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Message, b.Message)
	})

	groups := make([]skillWarningGroup, 0)
	for _, warning := range sorted {
		if len(groups) == 0 || groups[len(groups)-1].name != warning.Name || groups[len(groups)-1].path != warning.Path {
			groups = append(groups, skillWarningGroup{
				name: warning.Name,
				path: warning.Path,
			})
		}
		groups[len(groups)-1].messages = append(groups[len(groups)-1].messages, warning.Message)
	}

	return groups, len(sorted)
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
