package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

type Options struct {
	Getwd      func() (string, error)
	GetHomeDir func() (string, error)
	Stdout     io.Writer
	Stderr     io.Writer
}

func NewRootCmd(opts Options) *cobra.Command {
	if opts.Getwd == nil {
		opts.Getwd = os.Getwd
	}
	if opts.GetHomeDir == nil {
		opts.GetHomeDir = os.UserHomeDir
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}

	cmd := &cobra.Command{
		Use:           "ski",
		Short:         "Manage AI agent skills",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetOut(opts.Stdout)
	cmd.SetErr(opts.Stderr)

	cmd.AddCommand(newAddCmd(opts))
	cmd.AddCommand(newDoctorCmd(opts))
	cmd.AddCommand(newInitCmd(opts))
	cmd.AddCommand(newInstallCmd(opts))
	cmd.AddCommand(newListCmd(opts))
	cmd.AddCommand(newRemoveCmd(opts))
	cmd.AddCommand(newUpdateCmd(opts))
	return cmd
}
