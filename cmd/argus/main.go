package main

import (
	"embed"
	"os"

	"github.com/spf13/cobra"
	"github.com/ulm0/argus/cmd/argus/cmd"
)

//go:embed all:web/out
var webContent embed.FS

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	cmd.Version = version

	root := &cobra.Command{
		Use:   "argus",
		Short: "Edge dashcam manager for Tesla vehicles",
		Long:  "Argus manages Dashcam and Sentry Mode data on a Raspberry Pi Zero 2 W.",
	}

	root.AddCommand(
		cmd.NewRunCmd(&webContent),
		cmd.NewSetupCmd(),
		cmd.NewRemoveCmd(),
		cmd.NewUpgradeCmd(),
		cmd.NewVersionCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
