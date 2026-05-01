package cmd

import (
	"embed"
	"fmt"

	"github.com/spf13/cobra"
)

// NewRefreshServiceCmd re-renders /etc/systemd/system/argus.service from this
// binary's embedded template and runs `systemctl daemon-reload`. It does NOT
// restart the service so it is safe to call from inside the running service
// itself; the new unit takes effect on the next restart cycle.
func NewRefreshServiceCmd(templates *embed.FS) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh-service",
		Short: "Re-render /etc/systemd/system/argus.service from the embedded template",
		Long: `Re-renders the systemd unit file from this binary's embedded template,
preserving the existing WorkingDirectory (install location), then runs
'systemctl daemon-reload'. Use after upgrades to keep the unit in sync
when the template changes between versions. Must be run as root.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setupCheckRoot(); err != nil {
				return err
			}
			if err := RefreshServiceUnit(templates); err != nil {
				return fmt.Errorf("refresh service unit: %w", err)
			}
			if err := ensureSystemdReleasesWatchdog(); err != nil {
				return fmt.Errorf("systemd watchdog drop-in: %w", err)
			}
			fmt.Println("Refresh complete. Reboot if watchdog.service failed: PID1 must reload system.conf.d before /dev/watchdog is free for the Debian watchdog daemon.")
			return nil
		},
	}
}
