package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Options controls filesystem, IO, and TTY dependencies for the CLI.
type Options struct {
	Getwd             func() (string, error)
	GetHomeDir        func() (string, error)
	Stdin             io.Reader
	Stdout            io.Writer
	Stderr            io.Writer
	IsTTY             func() bool
	PromptMultiSelect func(req MultiSelectRequest) ([]string, error)
}

// NewRootCmd constructs the ski CLI with all implemented subcommands.
func NewRootCmd(opts Options) *cobra.Command {
	if opts.Getwd == nil {
		opts.Getwd = os.Getwd
	}
	if opts.GetHomeDir == nil {
		opts.GetHomeDir = os.UserHomeDir
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.IsTTY == nil {
		opts.IsTTY = func() bool {
			stdinInfo, err := os.Stdin.Stat()
			if err != nil || stdinInfo.Mode()&os.ModeCharDevice == 0 {
				return false
			}
			stdoutInfo, err := os.Stdout.Stat()
			if err != nil || stdoutInfo.Mode()&os.ModeCharDevice == 0 {
				return false
			}
			return true
		}
	}

	cmd := &cobra.Command{
		Use:           "ski",
		Short:         "Manage AI agent skills",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetIn(opts.Stdin)
	cmd.SetOut(opts.Stdout)
	cmd.SetErr(opts.Stderr)
	cmd.PersistentFlags().BoolP("global", "g", false, "Use the global manifest and global target scope")

	cmd.AddCommand(newAddCmd(opts))
	cmd.AddCommand(newDoctorCmd(opts))
	cmd.AddCommand(newInitCmd(opts))
	cmd.AddCommand(newInstallCmd(opts))
	cmd.AddCommand(newListCmd(opts))
	cmd.AddCommand(newRemoveCmd(opts))
	cmd.AddCommand(newUpdateCmd(opts))
	return cmd
}
