package config

import (
	"os"
	"path/filepath"
	"testing"
)

const minimalYAML = `
installation:
  target_user: pi
  mount_dir: /mnt/usb
disk_images:
  cam_name: usb_cam.img
  lightshow_name: usb_lightshow.img
  cam_label: CAM
  lightshow_label: LIGHTSHOW
  part2_enabled: true
  music_enabled: false
  boot_fsck_enabled: true
setup:
  part1_size: 32G
  part2_size: 4G
  part3_size: 2G
  reserve_size: 512M
network:
  samba_password: secret
web:
  max_lock_chime_size: 1048576
  max_lock_chime_duration: 10.0
  min_lock_chime_duration: 0.5
`

func writeTestConfig(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "gadget", "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	path := writeTestConfig(t, minimalYAML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Installation.TargetUser != "pi" {
		t.Errorf("TargetUser = %q, want %q", cfg.Installation.TargetUser, "pi")
	}
	if cfg.Installation.MountDir != "/mnt/usb" {
		t.Errorf("MountDir = %q, want %q", cfg.Installation.MountDir, "/mnt/usb")
	}
	if cfg.DiskImages.CamName != "usb_cam.img" {
		t.Errorf("CamName = %q, want %q", cfg.DiskImages.CamName, "usb_cam.img")
	}
	if cfg.Network.SambaPassword != "secret" {
		t.Errorf("SambaPassword = %q, want %q", cfg.Network.SambaPassword, "secret")
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	path := writeTestConfig(t, "{{{{not yaml")
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for invalid YAML, got nil")
	}
}

func TestDefaults(t *testing.T) {
	path := writeTestConfig(t, minimalYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"MusicName", cfg.DiskImages.MusicName, "usb_music.img"},
		{"MusicLabel", cfg.DiskImages.MusicLabel, "Music"},
		{"MusicFS", cfg.DiskImages.MusicFS, "fat32"},
		{"WebPort", cfg.Network.WebPort, 80},
		{"MaxUploadSizeMB", cfg.Web.MaxUploadSizeMB, 2048},
		{"MaxUploadChunkMB", cfg.Web.MaxUploadChunkMB, 16},
		{"LockChimeFilename", cfg.Web.LockChimeFilename, "LockChime.wav"},
		{"ChimesFolder", cfg.Web.ChimesFolder, "Chimes"},
		{"LightshowFolder", cfg.Web.LightshowFolder, "LightShow"},
		{"TelegramOfflineMode", cfg.Telegram.OfflineMode, "queue"},
		{"TelegramMaxQueueSize", cfg.Telegram.MaxQueueSize, 50},
		{"TelegramVideoQuality", cfg.Telegram.VideoQuality, "hd"},
		{"OfflineAPForceMode", cfg.OfflineAP.ForceMode, "auto"},
		{"OfflineAPVirtualInterface", cfg.OfflineAP.VirtualInterface, "uap0"},
		{"OfflineAPPingTarget", cfg.OfflineAP.PingTarget, "8.8.8.8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %v, want %v", tt.got, tt.want)
			}
		})
	}

	if cfg.Web.SecretKey == "" || cfg.Web.SecretKey == defaultSecretKey {
		t.Error("SecretKey should be auto-generated when empty")
	}
	if len(cfg.Web.SecretKey) != 64 {
		t.Errorf("SecretKey hex length = %d, want 64", len(cfg.Web.SecretKey))
	}
}

func TestDefaultOverride(t *testing.T) {
	yaml := minimalYAML + `
  web_port: 8080
`
	// network.web_port override needs correct nesting; use a full override config
	overrideYAML := `
installation:
  target_user: pi
  mount_dir: /mnt/usb
disk_images:
  cam_name: usb_cam.img
  lightshow_name: usb_lightshow.img
  music_name: custom_music.img
  music_label: MyMusic
  music_fs: ext4
network:
  samba_password: pw
  web_port: 8080
web:
  max_upload_size_mb: 512
  max_upload_chunk_mb: 8
  lock_chime_filename: CustomChime.wav
  chimes_folder: MyChimes
  lightshow_folder: MyLightShow
  max_lock_chime_size: 1048576
  max_lock_chime_duration: 10.0
  min_lock_chime_duration: 0.5
`
	_ = yaml
	path := writeTestConfig(t, overrideYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DiskImages.MusicName != "custom_music.img" {
		t.Errorf("MusicName = %q, want %q", cfg.DiskImages.MusicName, "custom_music.img")
	}
	if cfg.Network.WebPort != 8080 {
		t.Errorf("WebPort = %d, want 8080", cfg.Network.WebPort)
	}
	if cfg.Web.MaxUploadSizeMB != 512 {
		t.Errorf("MaxUploadSizeMB = %d, want 512", cfg.Web.MaxUploadSizeMB)
	}
	if cfg.Web.LockChimeFilename != "CustomChime.wav" {
		t.Errorf("LockChimeFilename = %q, want %q", cfg.Web.LockChimeFilename, "CustomChime.wav")
	}
}

func TestComputedPaths(t *testing.T) {
	path := writeTestConfig(t, minimalYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	gadgetDir := filepath.Dir(path)

	if cfg.GadgetDir != gadgetDir {
		t.Errorf("GadgetDir = %q, want %q", cfg.GadgetDir, gadgetDir)
	}
	if cfg.MountDir != "/mnt/usb" {
		t.Errorf("MountDir = %q, want %q", cfg.MountDir, "/mnt/usb")
	}
	if cfg.ImgCamPath != filepath.Join(gadgetDir, "usb_cam.img") {
		t.Errorf("ImgCamPath = %q, want %q", cfg.ImgCamPath, filepath.Join(gadgetDir, "usb_cam.img"))
	}
	if cfg.ImgLightshow != filepath.Join(gadgetDir, "usb_lightshow.img") {
		t.Errorf("ImgLightshow = %q, want %q", cfg.ImgLightshow, filepath.Join(gadgetDir, "usb_lightshow.img"))
	}
	if cfg.ImgMusicPath != filepath.Join(gadgetDir, "usb_music.img") {
		t.Errorf("ImgMusicPath = %q, want %q", cfg.ImgMusicPath, filepath.Join(gadgetDir, "usb_music.img"))
	}
	if cfg.StateFile != filepath.Join(gadgetDir, "state.txt") {
		t.Errorf("StateFile = %q, want %q", cfg.StateFile, filepath.Join(gadgetDir, "state.txt"))
	}
	if cfg.ThumbnailDir != filepath.Join(gadgetDir, "thumbnails") {
		t.Errorf("ThumbnailDir = %q, want %q", cfg.ThumbnailDir, filepath.Join(gadgetDir, "thumbnails"))
	}
	if cfg.ConfigFilePath != path {
		t.Errorf("ConfigFilePath = %q, want %q", cfg.ConfigFilePath, path)
	}
}

func TestUSBPartitionsMusicDisabled(t *testing.T) {
	path := writeTestConfig(t, minimalYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	parts := cfg.USBPartitions()
	want := []string{"part1", "part2"}
	if len(parts) != len(want) {
		t.Fatalf("USBPartitions() len = %d, want %d", len(parts), len(want))
	}
	for i, p := range parts {
		if p != want[i] {
			t.Errorf("USBPartitions()[%d] = %q, want %q", i, p, want[i])
		}
	}
}

func TestUSBPartitionsMusicEnabled(t *testing.T) {
	yaml := `
installation:
  target_user: pi
  mount_dir: /mnt/usb
disk_images:
  cam_name: usb_cam.img
  lightshow_name: usb_lightshow.img
  part2_enabled: true
  music_enabled: true
network:
  samba_password: pw
web:
  max_lock_chime_size: 1048576
  max_lock_chime_duration: 10.0
  min_lock_chime_duration: 0.5
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	parts := cfg.USBPartitions()
	want := []string{"part1", "part2", "part3"}
	if len(parts) != len(want) {
		t.Fatalf("USBPartitions() len = %d, want %d", len(parts), len(want))
	}
	for i, p := range parts {
		if p != want[i] {
			t.Errorf("USBPartitions()[%d] = %q, want %q", i, p, want[i])
		}
	}
}

func TestMountPath(t *testing.T) {
	path := writeTestConfig(t, minimalYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	tests := []struct {
		partition string
		readOnly  bool
		want      string
	}{
		{"part1", false, "/mnt/usb/part1"},
		{"part1", true, "/mnt/usb/part1-ro"},
		{"part2", false, "/mnt/usb/part2"},
		{"part2", true, "/mnt/usb/part2-ro"},
	}

	for _, tt := range tests {
		name := tt.partition
		if tt.readOnly {
			name += "-ro"
		}
		t.Run(name, func(t *testing.T) {
			got := cfg.MountPath(tt.partition, tt.readOnly)
			if got != tt.want {
				t.Errorf("MountPath(%q, %v) = %q, want %q", tt.partition, tt.readOnly, got, tt.want)
			}
		})
	}
}

func TestCameraAngles(t *testing.T) {
	path := writeTestConfig(t, minimalYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	angles := cfg.CameraAngles()
	want := []string{"front", "back", "left_repeater", "right_repeater", "left_pillar", "right_pillar"}

	if len(angles) != len(want) {
		t.Fatalf("CameraAngles() len = %d, want %d", len(angles), len(want))
	}
	for i, a := range angles {
		if a != want[i] {
			t.Errorf("CameraAngles()[%d] = %q, want %q", i, a, want[i])
		}
	}
}

func TestSaveAndReload(t *testing.T) {
	path := writeTestConfig(t, minimalYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg.Network.WebPort = 9999
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	cfg2, err := Load(path)
	if err != nil {
		t.Fatalf("Load() after save error: %v", err)
	}
	if cfg2.Network.WebPort != 9999 {
		t.Errorf("WebPort after save/reload = %d, want 9999", cfg2.Network.WebPort)
	}
}
