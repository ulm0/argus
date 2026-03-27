package cmd

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/ulm0/argus/internal/config"
)

const defaultConfigYAML = `installation:
  target_user: pi # replaced at runtime by the detected non-root user
  mount_dir: /mnt/gadget

disk_images:
  cam_name: usb_cam.img
  lightshow_name: usb_lightshow.img
  cam_label: TeslaCam
  lightshow_label: Lightshow
  music_name: usb_music.img
  music_label: Music
  part2_enabled: true      # set to false to skip the entire Chimes/LightShow/Wraps partition
  chimes_enabled: true     # custom lock/unlock sounds (lives in part2)
  lightshow_enabled: true  # light show sequences (lives in part2)
  wraps_enabled: true      # custom vehicle wraps (lives in part2)
  music_enabled: true      # set to false to skip the Music partition
  music_fs: fat32
  boot_fsck_enabled: true

setup:
  part2_size: ""    # LightShow/Chimes/Wraps image size (e.g. "10G"; default: 10G)
  part3_size: ""    # Music image size (e.g. "32G"; default: 32G)
  reserve_size: ""  # Free space to keep on Pi filesystem (default: 5G)

network:
  samba_password: tesla
  web_port: 80

offline_ap:
  enabled: true
  interface: wlan0
  ssid: Argus
  passphrase: argus1234
  channel: 6
  ipv4_cidr: 192.168.4.1/24
  dhcp_start: 192.168.4.10
  dhcp_end: 192.168.4.50
  check_interval: 20
  disconnect_grace: 30
  min_rssi: -70
  stable_seconds: 20
  ping_target: 8.8.8.8
  retry_seconds: 300
  virtual_interface: uap0
  force_mode: auto

system:
  config_file: /boot/firmware/config.txt
  samba_conf: /etc/samba/smb.conf

web:
  secret_key: CHANGE-THIS-TO-A-RANDOM-SECRET-KEY-ON-FIRST-INSTALL
  max_lock_chime_size: 1048576
  max_lock_chime_duration: 10.0
  min_lock_chime_duration: 0.3
  speed_range_min: 0.5
  speed_range_max: 2.0
  speed_step: 0.05
  lock_chime_filename: LockChime.wav
  chimes_folder: Chimes
  lightshow_folder: LightShow
  max_upload_size_mb: 2048
  max_upload_chunk_mb: 16

telegram:
  enabled: false
  bot_token: ""
  chat_id: ""
  offline_mode: queue
  max_queue_size: 50
  video_quality: hd

update:
  auto_update: false
  check_on_startup: true
  channel: stable
`

func NewSetupCmd(templates *embed.FS) *cobra.Command {
	var (
		installDir string
		cfgPath    string
		showSize   string
		musicSize  string
	)

	c := &cobra.Command{
		Use:   "setup",
		Short: "Install Argus on a Raspberry Pi",
		Long: `Install Argus: create disk images, configure boot parameters,
swap, sysctl, and install the systemd service.
Must be run as root (sudo).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			setupPrintBanner()

			if err := setupCheckRoot(); err != nil {
				return err
			}

			arch, err := setupDetectArch()
			if err != nil {
				return err
			}
			setupLog("Detected architecture: %s", arch)

			if installDir == "" {
				installDir = setupDefaultDir()
			}
			if cfgPath == "" {
				cfgPath = filepath.Join(installDir, "config.yaml")
			}

			if err := setupInstallDeps(); err != nil {
				return err
			}
			if err := os.MkdirAll(installDir, 0755); err != nil {
				return fmt.Errorf("create install dir: %w", err)
			}
			if err := setupWriteConfig(cfgPath); err != nil {
				return err
			}
			cfg, err := loadSetupConfig(cfgPath)
			if err != nil {
				return err
			}
			if err := setupDiskImages(installDir, showSize, musicSize, cfg); err != nil {
				return err
			}
			if err := setupSeedFolders(installDir, cfg); err != nil {
				return err
			}
			if err := setupConfigureBoot(); err != nil {
				return err
			}
			if err := setupConfigureSwap(); err != nil {
				return err
			}
			if err := setupConfigureSysctl(); err != nil {
				return err
			}
			if err := setupInstallService(installDir, templates); err != nil {
				return err
			}
			setupMaskDesktopServices()

		fmt.Println()
		setupLog("Setup complete!")
		fmt.Printf("\n  Config:  %s\n", cfgPath)
		fmt.Printf("  Data:    %s\n", installDir)
		fmt.Printf("  Binary:  /usr/local/bin/argus\n")
		fmt.Printf("  Service: argus.service\n\n")
		fmt.Println("  Edit config.yaml to set your Samba password, WiFi AP credentials, etc.")
		fmt.Println()
		setupReboot()
			return nil
		},
	}

	c.Flags().StringVarP(&cfgPath, "config", "c", "", "path to write config.yaml (default: <dir>/config.yaml)")
	c.Flags().StringVar(&installDir, "dir", "", "data directory (default: ~/.argus/)")
	c.Flags().StringVar(&showSize, "show-size", "", "LightShow image size (default: 10G, env: ARGUS_SHOW_SIZE)")
	c.Flags().StringVar(&musicSize, "music-size", "", "Music image size (default: 32G, env: ARGUS_MUSIC_SIZE)")
	return c
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func setupPrintBanner() {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  Argus - Setup")
	fmt.Println("========================================")
	fmt.Println()
}

func setupLog(format string, a ...any) {
	fmt.Printf("[+] "+format+"\n", a...)
}

func setupWarn(format string, a ...any) {
	fmt.Printf("[!] "+format+"\n", a...)
}

func setupCheckRoot() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("setup must be run as root (use sudo)")
	}
	return nil
}

func setupDetectArch() (string, error) {
	switch runtime.GOARCH {
	case "arm64":
		return "arm64", nil
	case "arm":
		return "arm", nil
	case "amd64":
		return "amd64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
}

// currentUser returns the real (non-root) user: $SUDO_USER, then $USER,
// then the OS current user, in that order.
func currentUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" {
		return u
	}
	if u := os.Getenv("USER"); u != "" && u != "root" {
		return u
	}
	if u, err := user.Current(); err == nil && u.Username != "root" {
		return u.Username
	}
	// Last resort: read the first non-root user from /etc/passwd
	if data, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) >= 7 && fields[0] != "root" && strings.HasPrefix(fields[6], "/bin/") {
				return fields[0]
			}
		}
	}
	return "pi"
}

func setupDefaultDir() string {
	targetUser := currentUser()
	u, err := user.Lookup(targetUser)
	if err == nil {
		return filepath.Join(u.HomeDir, ".argus")
	}
	return "/home/" + targetUser + "/.argus"
}

func setupInstallDeps() error {
	setupLog("Installing system dependencies...")
	packages := []string{
		"samba", "hostapd", "dnsmasq", "ffmpeg",
		"watchdog", "exfat-fuse", "exfatprogs",
		"dosfstools", "network-manager",
	}
	aptArgs := append([]string{"install", "-y", "-qq"}, packages...)
	if err := runCmd("apt-get", "update", "-qq"); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}
	if err := runCmd("apt-get", aptArgs...); err != nil {
		return fmt.Errorf("apt-get install: %w", err)
	}
	setupLog("Dependencies installed")
	return nil
}

// loadSetupConfig loads the config file written during setup.
// If it fails, it returns a config with all features enabled (safe defaults).
func loadSetupConfig(cfgPath string) (*config.Config, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config after write: %w", err)
	}
	return cfg, nil
}

func setupWriteConfig(cfgPath string) error {
	if _, err := os.Stat(cfgPath); err == nil {
		setupLog("Config already exists at %s", cfgPath)
		return nil
	}
	setupLog("Creating default config.yaml...")
	content := strings.Replace(defaultConfigYAML, "target_user: pi", "target_user: "+currentUser(), 1)
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	setupLog("Config created at %s", cfgPath)
	return nil
}

func setupDiskImages(installDir, showSize, musicSize string, cfg *config.Config) error {
	setupLog("Creating disk images...")
	if showSize == "" {
		if v := os.Getenv("ARGUS_SHOW_SIZE"); v != "" {
			showSize = v
		} else {
			showSize = "10G"
		}
	}
	if musicSize == "" {
		if v := os.Getenv("ARGUS_MUSIC_SIZE"); v != "" {
			musicSize = v
		} else {
			musicSize = "32G"
		}
	}

	reserveSize := cfg.Setup.ReserveSize
	if reserveSize == "" {
		reserveSize = "5G"
	}

	// Compute TeslaCam size: available space minus reserve and any enabled optional images.
	var optionalBytes int64
	if cfg.DiskImages.Part2Enabled {
		optionalBytes += parseSizeToBytes(showSize)
	}
	if cfg.DiskImages.MusicEnabled {
		optionalBytes += parseSizeToBytes(musicSize)
	}
	camSize, err := computeCamSize(installDir, reserveSize, optionalBytes)
	if err != nil {
		return fmt.Errorf("compute TeslaCam size: %w", err)
	}
	setupLog("TeslaCam image size: %s (remaining space)", camSize)

	type imgDef struct {
		name    string
		size    string
		fsType  string // "exfat" or "vfat"
		label   string
		warnMsg string
		enabled bool
	}

	images := []imgDef{
		{
			name:    "usb_cam.img",
			size:    camSize,
			fsType:  "exfat",
			label:   "TeslaCam",
			warnMsg: "TeslaCam image already exists, skipping",
			enabled: true,
		},
		{
			name:    "usb_lightshow.img",
			size:    showSize,
			fsType:  "vfat",
			label:   "Lightshow",
			warnMsg: "LightShow image already exists, skipping",
			enabled: cfg.DiskImages.Part2Enabled,
		},
		{
			name:    "usb_music.img",
			size:    musicSize,
			fsType:  "vfat",
			label:   "Music",
			warnMsg: "Music image already exists, skipping",
			enabled: cfg.DiskImages.MusicEnabled,
		},
	}

	for _, img := range images {
		if !img.enabled {
			setupLog("Skipping %s image (disabled in config)", img.label)
			continue
		}

		imgPath := filepath.Join(installDir, img.name)
		if _, err := os.Stat(imgPath); err == nil {
			setupWarn(img.warnMsg)
			continue
		}

		setupLog("Creating %s image (%s)...", img.label, img.size)
		if err := createSparseFile(imgPath, img.size); err != nil {
			return fmt.Errorf("create %s: %w", img.name, err)
		}

		switch img.fsType {
		case "exfat":
			if err := runCmd("mkfs.exfat", "-n", img.label, imgPath); err != nil {
				return fmt.Errorf("mkfs.exfat %s: %w", img.name, err)
			}
		case "vfat":
			if err := runCmd("mkfs.vfat", "-F", "32", "-n", img.label, imgPath); err != nil {
				return fmt.Errorf("mkfs.vfat %s: %w", img.name, err)
			}
		}
	}

	setupLog("Disk images created")
	return nil
}

func setupSeedFolders(installDir string, cfg *config.Config) error {
	setupLog("Seeding TeslaCam and Chime folders...")

	mntBase := "/mnt/gadget"

	// Always create part1 (TeslaCam - always required)
	if err := os.MkdirAll(filepath.Join(mntBase, "part1"), 0755); err != nil {
		return fmt.Errorf("mkdir part1: %w", err)
	}

	if cfg.DiskImages.Part2Enabled {
		if err := os.MkdirAll(filepath.Join(mntBase, "part2"), 0755); err != nil {
			return fmt.Errorf("mkdir part2: %w", err)
		}
	}

	if cfg.DiskImages.MusicEnabled {
		if err := os.MkdirAll(filepath.Join(mntBase, "part3"), 0755); err != nil {
			return fmt.Errorf("mkdir part3: %w", err)
		}
	}

	// Cam image
	camImg := filepath.Join(installDir, "usb_cam.img")
	if _, err := os.Stat(camImg); err == nil {
		if err := mountAndSeed(camImg, filepath.Join(mntBase, "part1"), []string{
			"TeslaCam/SavedClips",
			"TeslaCam/SentryClips",
			"TeslaCam/RecentClips",
		}); err != nil {
			return fmt.Errorf("seed cam: %w", err)
		}
	}

	// Lightshow image (only if part2 is enabled)
	if cfg.DiskImages.Part2Enabled {
		lsImg := filepath.Join(installDir, "usb_lightshow.img")
		if _, err := os.Stat(lsImg); err == nil {
			seeds := []string{}
			if cfg.DiskImages.ChimesEnabled {
				seeds = append(seeds, "Chimes")
			}
			if cfg.DiskImages.LightshowEnabled {
				seeds = append(seeds, "LightShow")
			}
			if cfg.DiskImages.WrapsEnabled {
				seeds = append(seeds, "Wraps")
			}
			if len(seeds) > 0 {
				if err := mountAndSeed(lsImg, filepath.Join(mntBase, "part2"), seeds); err != nil {
					return fmt.Errorf("seed lightshow: %w", err)
				}
			}
		}
	}

	// Music image (only if enabled)
	if cfg.DiskImages.MusicEnabled {
		musicImg := filepath.Join(installDir, "usb_music.img")
		if _, err := os.Stat(musicImg); err == nil {
			if err := mountAndSeed(musicImg, filepath.Join(mntBase, "part3"), []string{
				"Music",
			}); err != nil {
				return fmt.Errorf("seed music: %w", err)
			}
		}
	}

	setupLog("Folders seeded")
	return nil
}

func setupConfigureBoot() error {
	setupLog("Configuring boot parameters...")
	bootConfig := "/boot/firmware/config.txt"

	params := []string{
		"dtoverlay=dwc2",
		"dtparam=watchdog=on",
		"gpu_mem=16",
	}
	for _, p := range params {
		if err := appendLineIfMissing(bootConfig, p); err != nil {
			setupWarn("boot config %q: %v", p, err)
		}
	}

	for _, mod := range []string{"dwc2", "libcomposite"} {
		if err := appendLineIfMissing("/etc/modules", mod); err != nil {
			setupWarn("modules %s: %v", mod, err)
		}
	}

	setupLog("Boot configuration updated")
	return nil
}

func setupConfigureSwap() error {
	setupLog("Configuring swap...")
	swapFile := "/var/argus-fsck.swap"

	// Disable the OS default swap to avoid duplicate swap files on the same device.
	// dphys-swapfile may not be installed on all distros — silence errors silently.
	runCmdSilent("systemctl", "disable", "--now", "dphys-swapfile")
	runCmdSilent("dphys-swapfile", "swapoff")

	if _, err := os.Stat(swapFile); os.IsNotExist(err) {
		if err := runCmd("fallocate", "-l", "1G", swapFile); err != nil {
			return fmt.Errorf("fallocate: %w", err)
		}
		if err := os.Chmod(swapFile, 0600); err != nil {
			return fmt.Errorf("chmod swap: %w", err)
		}
		if err := runCmd("mkswap", swapFile); err != nil {
			return fmt.Errorf("mkswap: %w", err)
		}
	}

	fstabLine := swapFile + " none swap sw 0 0"
	if err := appendLineIfMissing("/etc/fstab", fstabLine); err != nil {
		setupWarn("fstab swap: %v", err)
	}

	_ = runCmd("swapon", "-a")
	setupLog("Swap configured")
	return nil
}

func setupConfigureSysctl() error {
	setupLog("Configuring sysctl...")
	content := "kernel.panic=10\nvm.swappiness=10\nvm.dirty_ratio=10\nvm.dirty_background_ratio=5\n"
	if err := os.WriteFile("/etc/sysctl.d/99-argus.conf", []byte(content), 0644); err != nil {
		return fmt.Errorf("write sysctl conf: %w", err)
	}
	_ = runCmd("sysctl", "-p", "/etc/sysctl.d/99-argus.conf")
	return nil
}

func setupInstallService(installDir string, templates *embed.FS) error {
	setupLog("Installing systemd service...")

	targetUser := currentUser()

	// Read the service template from the embedded FS; it is always present in the binary.
	data, err := templates.ReadFile("templates/argus.service")
	if err != nil {
		return fmt.Errorf("read service template: %w", err)
	}

	rendered := strings.ReplaceAll(string(data), "__GADGET_DIR__", installDir)
	rendered = strings.ReplaceAll(rendered, "__TARGET_USER__", targetUser)
	rendered = strings.ReplaceAll(rendered, "__MNT_DIR__", "/mnt/gadget")

	if err := os.WriteFile("/etc/systemd/system/argus.service", []byte(rendered), 0644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	if err := setupCopyBinary(); err != nil {
		return err
	}

	if err := runCmd("systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	if err := runCmd("systemctl", "enable", "argus.service"); err != nil {
		return fmt.Errorf("enable service: %w", err)
	}

	setupLog("Service installed and enabled")
	return nil
}

func setupCopyBinary() error {
	const binDest = "/usr/local/bin/argus"

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	// Write to a sibling temp file first, then atomically rename over binDest.
	// Opening binDest directly with O_TRUNC would fail with ETXTBSY on Linux
	// when the binary is already running from that path.
	tmp := binDest + ".new"

	src, err := os.Open(exe)
	if err != nil {
		return fmt.Errorf("open binary: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("stage binary at %s: %w", tmp, err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(tmp)
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		os.Remove(tmp)
		return fmt.Errorf("sync binary: %w", err)
	}
	dst.Close()

	if err := os.Rename(tmp, binDest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("install binary to %s: %w", binDest, err)
	}

	setupLog("Binary installed at %s", binDest)
	return nil
}
func setupMaskDesktopServices() {
	for _, svc := range []string{"pipewire", "pipewire-pulse", "wireplumber", "colord"} {
		runCmdSilent("systemctl", "--user", "mask", svc+".service")
	}
}

// setupReboot counts down 5 seconds and reboots unless the user presses Ctrl+C.
func setupReboot() {
	fmt.Println("  Rebooting in 5 seconds — press Ctrl+C to cancel and reboot manually.")
	fmt.Println()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)

	for i := 5; i > 0; i-- {
		fmt.Printf("\r  Rebooting in %d...  ", i)
		select {
		case <-sig:
			fmt.Println("\n\n  Reboot cancelled.")
			fmt.Println("  Start Argus manually with: sudo systemctl start argus")
			fmt.Println("  Or reboot when ready:      sudo reboot")
			fmt.Println()
			return
		case <-time.After(time.Second):
		}
	}

	fmt.Println("\r  Rebooting now...       ")
	_ = exec.Command("systemctl", "reboot").Run()
}

// ─── shared utilities ────────────────────────────────────────────────────────

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCmdSilent runs a command discarding all output. Used for best-effort
// operations where the command may not exist (e.g. dphys-swapfile).
func runCmdSilent(name string, args ...string) {
	cmd := exec.Command(name, args...)
	_ = cmd.Run()
}

func createSparseFile(path, size string) error {
	return runCmd("truncate", "-s", size, path)
}

func appendLineIfMissing(filePath, line string) error {
	data, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) == line {
			return nil
		}
	}
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}

// computeCamSize calculates how many GiB to allocate for the TeslaCam image.
// It takes available filesystem space, subtracts the reserve and the sum of
// already-accounted optional image sizes, and returns a size string like "47G".
func computeCamSize(installDir, reserveSize string, optionalBytes int64) (string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(installDir, &stat); err != nil {
		return "", fmt.Errorf("statfs %s: %w", installDir, err)
	}
	availBytes := int64(stat.Bavail) * int64(stat.Bsize)
	reserveBytes := parseSizeToBytes(reserveSize)
	camBytes := availBytes - reserveBytes - optionalBytes
	if camBytes <= 0 {
		return "", fmt.Errorf("not enough free space: available=%dG reserve=%dG optional=%dG",
			availBytes/1024/1024/1024,
			reserveBytes/1024/1024/1024,
			optionalBytes/1024/1024/1024,
		)
	}
	return fmt.Sprintf("%dG", camBytes/1024/1024/1024), nil
}

// parseSizeToBytes converts a human-readable size string (e.g. "10G", "512M") to bytes.
// Supported suffixes: G/g (GiB), M/m (MiB), K/k (KiB). Bare numbers are treated as bytes.
func parseSizeToBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	multiplier := int64(1)
	switch s[len(s)-1] {
	case 'G', 'g':
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case 'K', 'k':
		multiplier = 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n * multiplier
}

func mountAndSeed(imgPath, mntPoint string, dirs []string) error {
	// Attach loop device
	out, err := exec.Command("losetup", "--show", "-f", imgPath).Output()
	if err != nil {
		return fmt.Errorf("losetup: %w", err)
	}
	loopDev := strings.TrimSpace(string(out))
	defer func() {
		_ = runCmd("losetup", "-d", loopDev)
	}()

	if err := runCmd("mount", loopDev, mntPoint); err != nil {
		return fmt.Errorf("mount %s: %w", loopDev, err)
	}
	defer func() {
		_ = runCmd("umount", mntPoint)
	}()

	for _, d := range dirs {
		fullPath := filepath.Join(mntPoint, d)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", fullPath, err)
		}
	}
	return nil
}

func promptYesNo(question string) bool {
	fmt.Print(question + " [y/N] ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		reply := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return reply == "y" || reply == "yes"
	}
	return false
}
