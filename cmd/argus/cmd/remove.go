package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func NewRemoveCmd() *cobra.Command {
	var (
		cfgPath    string
		yes        bool
		keepImages bool
	)

	c := &cobra.Command{
		Use:   "remove",
		Short: "Uninstall Argus from this system",
		Long: `Stop and remove the Argus service, USB gadget, mounts, disk configuration,
system files and state. Disk image files are preserved by default.
Must be run as root (sudo).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			removePrintBanner()

			if err := setupCheckRoot(); err != nil {
				return err
			}

			installDir := resolveInstallDir(cfgPath)

			fmt.Println("This will remove the Argus service and configuration.")
			fmt.Println("Disk images will be preserved unless you choose to remove them.")
			fmt.Println()
			if !yes && !promptYesNo("Continue?") {
				fmt.Println("Aborted.")
				return nil
			}

			removeLog("Step 1/7: Removing systemd service...")
			removeService()

			removeLog("Step 2/7: Cleaning USB gadget...")
			removeGadget()

			removeLog("Step 3/7: Unmounting partitions...")
			removeUnmountPartitions(installDir)

			removeLog("Step 4/7: Cleaning Samba configuration...")
			removeSamba()

			removeLog("Step 5/7: Removing system configurations...")
			removeSystemConfigs()

			removeLog("Step 6/7: Cleaning swap...")
			removeSwap()

			removeLog("Step 7/7: Cleaning up state files...")
			removeStateFiles(installDir)

			fmt.Println()

			if !keepImages {
				removeImages(installDir, yes)
			}

			fmt.Println()
			removeLog("Cleanup complete!")
			fmt.Println()
			fmt.Printf("  You may also want to:\n")
			fmt.Printf("    - Remove the Argus directory: rm -rf %s\n", installDir)
			fmt.Printf("    - Uninstall packages: sudo apt remove samba hostapd dnsmasq\n")
			fmt.Printf("    - Reboot to fully clear the USB gadget kernel module\n")
			fmt.Println()
			return nil
		},
	}

	c.Flags().StringVarP(&cfgPath, "config", "c", "", "path to config.yaml (used to determine install directory)")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "skip all confirmation prompts")
	c.Flags().BoolVar(&keepImages, "keep-images", false, "do not offer to delete disk image files")
	return c
}

// ─── step helpers ────────────────────────────────────────────────────────────

func removePrintBanner() {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  Argus - Remove")
	fmt.Println("========================================")
	fmt.Println()
}

func removeLog(format string, a ...any) {
	fmt.Printf("[+] "+format+"\n", a...)
}

func removeWarn(format string, a ...any) {
	fmt.Printf("[!] "+format+"\n", a...)
}

func removeService() {
	_ = runCmd("systemctl", "stop", "argus.service")
	_ = runCmd("systemctl", "disable", "argus.service")
	_ = os.Remove("/etc/systemd/system/argus.service")
	_ = runCmd("systemctl", "daemon-reload")
	removeLog("Service removed")
}

func removeGadget() {
	gadgetDir := "/sys/kernel/config/usb_gadget/argus"
	if _, err := os.Stat(gadgetDir); os.IsNotExist(err) {
		return
	}

	// Detach from UDC
	udcPath := filepath.Join(gadgetDir, "UDC")
	_ = os.WriteFile(udcPath, []byte(""), 0644)

	// Remove symlinks in configs
	configsDir := filepath.Join(gadgetDir, "configs")
	_ = removeSymlinks(configsDir)

	// Remove subdirs in reverse order
	for _, rel := range []string{
		"configs/c.1/strings/0x409",
		"configs/c.1",
		"strings/0x409",
	} {
		_ = os.Remove(filepath.Join(gadgetDir, rel))
	}

	// Remove LUN dirs
	funcsDir := filepath.Join(gadgetDir, "functions", "mass_storage.usb0")
	if entries, err := os.ReadDir(funcsDir); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "lun.") {
				_ = os.Remove(filepath.Join(funcsDir, e.Name()))
			}
		}
	}
	_ = os.Remove(funcsDir)
	_ = os.Remove(filepath.Join(gadgetDir, "functions"))
	_ = os.Remove(gadgetDir)

	removeLog("Gadget cleaned")
}

func removeSymlinks(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
		if e.IsDir() {
			_ = removeSymlinks(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

func removeUnmountPartitions(installDir string) {
	mntBase := "/mnt/gadget"
	for _, part := range []string{"part1", "part2", "part3"} {
		for _, suffix := range []string{"", "-ro"} {
			mnt := filepath.Join(mntBase, part+suffix)
			if isMounted(mnt) {
				_ = runCmd("umount", "-l", mnt)
			}
		}
	}

	for _, img := range []string{"usb_cam.img", "usb_lightshow.img", "usb_music.img"} {
		imgPath := filepath.Join(installDir, img)
		if _, err := os.Stat(imgPath); err == nil {
			detachLoopDevices(imgPath)
		}
	}
	removeLog("Partitions unmounted")
}

func removeSamba() {
	smb := "/etc/samba/smb.conf"
	if _, err := os.Stat(smb); os.IsNotExist(err) {
		return
	}
	_ = runCmd("sed", "-i",
		`/\[gadget_part/,/^\[/{ /^\[gadget_part/d; /^\[/!d; }`,
		smb)
	_ = runCmd("smbpasswd", "-x", currentUser())
	removeLog("Samba cleaned")
}

func removeSystemConfigs() {
	_ = os.Remove("/etc/sysctl.d/99-argus.conf")
	_ = os.Remove("/etc/NetworkManager/conf.d/wifi-roaming.conf")

	bootConfig := "/boot/firmware/config.txt"
	for _, param := range []string{"dtoverlay=dwc2", "dtparam=watchdog=on", "gpu_mem=16"} {
		_ = runCmd("sed", "-i", "/"+param+"/d", bootConfig)
	}
	_ = runCmd("sed", "-i", "/dwc2/d", "/etc/modules")
	removeLog("System configs removed")
}

func removeSwap() {
	swapFile := "/var/swap/fsck.swap"
	if _, err := os.Stat(swapFile); err == nil {
		_ = runCmd("swapoff", swapFile)
		_ = os.Remove(swapFile)
		_ = runCmd("sed", "-i", `\|`+swapFile+`|d`, "/etc/fstab")
	}
	removeLog("Swap cleaned")
}

func removeStateFiles(installDir string) {
	_ = os.RemoveAll("/mnt/gadget")

	stateFiles := []string{
		filepath.Join(installDir, "state.txt"),
		filepath.Join(installDir, "fsck_status.json"),
		filepath.Join(installDir, "fsck_history.json"),
		filepath.Join(installDir, "chime_schedules.json"),
		filepath.Join(installDir, "chime_groups.json"),
		filepath.Join(installDir, "chime_random_config.json"),
		filepath.Join(installDir, "cleanup_config.json"),
		"/tmp/argus_wifi_status.json",
	}
	for _, f := range stateFiles {
		_ = os.Remove(f)
	}

	// Remove lock files
	if matches, err := filepath.Glob(filepath.Join(installDir, ".quick_edit_part*.lock")); err == nil {
		for _, m := range matches {
			_ = os.Remove(m)
		}
	}

	_ = os.RemoveAll(filepath.Join(installDir, "thumbnails"))
	_ = os.RemoveAll("/run/argus-ap")

	removeLog("Files cleaned")
}

func removeImages(installDir string, yes bool) {
	if !yes && !promptYesNo("Remove disk images? This DELETES ALL DASHCAM DATA!") {
		return
	}
	for _, img := range []string{"usb_cam.img", "usb_lightshow.img", "usb_music.img"} {
		imgPath := filepath.Join(installDir, img)
		if info, err := os.Stat(imgPath); err == nil {
			if err := os.Remove(imgPath); err == nil {
				removeLog("Removed %s (%s)", img, formatBytes(info.Size()))
			}
		}
	}
}

// ─── low-level utilities ─────────────────────────────────────────────────────

func resolveInstallDir(cfgPath string) string {
	if cfgPath != "" {
		return filepath.Dir(cfgPath)
	}
	if p := os.Getenv("ARGUS_CONFIG"); p != "" {
		return filepath.Dir(p)
	}
	return setupDefaultDir()
}

func isMounted(path string) bool {
	cmd := exec.Command("mountpoint", "-q", path)
	return cmd.Run() == nil
}

func detachLoopDevices(imgPath string) {
	out, err := exec.Command("losetup", "-j", imgPath).Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if dev := strings.SplitN(line, ":", 2)[0]; dev != "" {
			_ = runCmd("losetup", "-d", dev)
		}
	}
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n := n / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
