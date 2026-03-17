package cli

import (
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
)

// MultiSelectRequest describes a CLI multi-select prompt.
type MultiSelectRequest struct {
	Title       string
	Description string
	Options     []string
	MinSelected int
}

func runMultiSelectPrompt(opts Options, req MultiSelectRequest) ([]string, error) {
	if opts.PromptMultiSelect != nil {
		return opts.PromptMultiSelect(req)
	}
	return runHuhMultiSelectPrompt(opts.Stdin, opts.Stdout, req)
}

func runHuhMultiSelectPrompt(input io.Reader, output io.Writer, req MultiSelectRequest) ([]string, error) {
	selected := make([]string, 0, len(req.Options))
	field := huh.NewMultiSelect[string]().
		Title(req.Title).
		Value(&selected).
		Options(huh.NewOptions(req.Options...)...).
		Filterable(len(req.Options) > 8).
		Height(promptHeight(len(req.Options)))
	if req.Description != "" {
		field = field.Description(req.Description)
	}
	if req.MinSelected > 0 {
		field = field.Validate(func(values []string) error {
			if len(values) < req.MinSelected {
				if req.MinSelected == 1 {
					return fmt.Errorf("select at least one option")
				}
				return fmt.Errorf("select at least %d options", req.MinSelected)
			}
			return nil
		})
	}

	form := huh.NewForm(huh.NewGroup(field)).
		WithInput(input).
		WithOutput(output)
	if err := form.Run(); err != nil {
		return nil, err
	}
	return normalizeSelections(selected, req.Options), nil
}

func normalizeSelections(selected, options []string) []string {
	if len(selected) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(selected))
	for _, value := range selected {
		seen[value] = struct{}{}
	}

	out := make([]string, 0, len(selected))
	for _, value := range options {
		if _, ok := seen[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func promptHeight(numOptions int) int {
	switch {
	case numOptions <= 4:
		return numOptions
	case numOptions <= 8:
		return 6
	default:
		return 8
	}
}
