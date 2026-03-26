package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/ulm0/argus/cmd/argus/cmd.Version=..."
var Version = "dev"

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Argus version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("argus", Version)
		},
	}
}
