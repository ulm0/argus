package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/ulm0/argus/internal/updater"
)

func NewUpgradeCmd() *cobra.Command {
	var yes bool

	c := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade Argus to the latest release",
		Long: `Check GitHub for the latest release asset (a raw argus binary),
download it, replace /usr/local/bin/argus atomically, refresh the systemd
unit from the new binary's embedded template, and restart argus.service.
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

			// Ask the freshly installed binary to refresh the systemd unit
			// from its (new) embedded template. We do this with the new
			// binary because the template lives inside it; the currently
			// running binary still has the old version embedded.
			fmt.Println("[+] Refreshing systemd unit from new template...")
			if out, err := exec.Command("/usr/local/bin/argus", "refresh-service").CombinedOutput(); err != nil {
				fmt.Printf("    warning: refresh-service failed: %v\n%s\n", err, string(out))
				fmt.Println("    Run `sudo argus refresh-service` manually to clean up the unit.")
			} else {
				_ = exec.Command("systemctl", "restart", "argus.service").Run()
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
