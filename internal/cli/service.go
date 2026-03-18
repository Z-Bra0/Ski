package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Z-Bra0/Ski/internal/app"
	"github.com/Z-Bra0/Ski/internal/manifest"
)

func newService(cmd *cobra.Command, opts Options) (app.Service, error) {
	cwd, err := opts.Getwd()
	if err != nil {
		return app.Service{}, fmt.Errorf("resolve working directory: %w", err)
	}
	homeDir, err := opts.GetHomeDir()
	if err != nil {
		return app.Service{}, fmt.Errorf("resolve home directory: %w", err)
	}
	global, err := cmd.Flags().GetBool("global")
	if err != nil {
		return app.Service{}, fmt.Errorf("read global flag: %w", err)
	}

	return app.Service{ProjectDir: cwd, HomeDir: homeDir, Global: global}, nil
}

func manifestDisplayName(svc app.Service) string {
	if svc.Global {
		return manifest.GlobalPath(svc.HomeDir)
	}
	return manifest.FileName
}
