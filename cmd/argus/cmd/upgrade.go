package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ulm0/argus/internal/updater"
)

func NewUpgradeCmd() *cobra.Command {
	var yes bool

	c := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade Argus to the latest release",
		Long: `Check GitHub for a newer release and install it.
The binary is atomically replaced and the systemd service is restarted.
Must be run as root (sudo).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setupCheckRoot(); err != nil {
				return err
			}

			fmt.Println()
			fmt.Println("========================================")
			fmt.Println("  Argus - Upgrade")
			fmt.Println("========================================")
			fmt.Println()

			fmt.Printf("[+] Current version: %s\n", Version)

			if !updater.IsOnline() {
				return fmt.Errorf("no internet connection — cannot reach api.github.com")
			}

			fmt.Println("[+] Checking for latest release...")
			release, err := updater.CheckLatest(Version)
			if err != nil {
				return fmt.Errorf("check latest: %w", err)
			}
			if release == nil {
				fmt.Println("[+] Already up to date.")
				return nil
			}

			fmt.Printf("[+] New version available: %s (published %s)\n",
				release.Version, release.PublishedAt.Format("2006-01-02"))
			fmt.Println()

			if !yes && !promptYesNo("Install update?") {
				fmt.Println("Aborted.")
				return nil
			}

			fmt.Println("[+] Downloading and installing...")
			if err := updater.Install(release); err != nil {
				return fmt.Errorf("install: %w", err)
			}

			fmt.Println()
			fmt.Printf("[+] Upgrade complete! Now running %s.\n", release.Version)
			fmt.Println("    The service has been restarted automatically.")
			fmt.Println()
			return nil
		},
	}

	c.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return c
}
