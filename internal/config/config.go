package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Installation InstallationConfig `yaml:"installation"`
	DiskImages   DiskImagesConfig   `yaml:"disk_images"`
	Setup        SetupConfig        `yaml:"setup"`
	Network      NetworkConfig      `yaml:"network"`
	OfflineAP    OfflineAPConfig    `yaml:"offline_ap"`
	System       SystemConfig       `yaml:"system"`
	Web          WebConfig          `yaml:"web"`
	Telegram     TelegramConfig     `yaml:"telegram"`
	Webhook      WebhookConfig      `yaml:"webhook"`
	Update       UpdateConfig       `yaml:"update"`
	ViewerPrefs  ViewerPrefsConfig  `yaml:"viewer_prefs"`
	LogLevel     string             `yaml:"log_level"`

	// Computed paths (not from YAML)
	GadgetDir      string `yaml:"-"`
	ImgCamPath     string `yaml:"-"`
	ImgLightshow   string `yaml:"-"`
	ImgMusicPath   string `yaml:"-"`
	StateFile      string `yaml:"-"`
	ThumbnailDir   string `yaml:"-"`
	MountDir       string `yaml:"-"`
	ConfigFilePath string `yaml:"-"`

	// saveMu serializes Save() so concurrent callers don't write the same
	// ".tmp" file at once (which would corrupt the persisted config) and don't
	// race each other's marshal/rename. Unexported, so yaml ignores it.
	saveMu sync.Mutex `yaml:"-"`
}

type InstallationConfig struct {
	TargetUser string `yaml:"target_user"`
	MountDir   string `yaml:"mount_dir"`
	// ArchivePath is the optional path to the archive storage directory.
	// If set and a TeslaCam subdirectory exists there, its contents are included in the folder listing.
	ArchivePath string `yaml:"archive_path"`
	// BootPresentOnStart runs SwitchToPresent once at process start (replaces TeslaUSB present_usb_on_boot.service).
	BootPresentOnStart bool `yaml:"boot_present_on_start"`
	// BootCleanupOnStart runs cleanup against TeslaCam before presenting when cleanup_config.json has boot_cleanup policies.
	BootCleanupOnStart bool `yaml:"boot_cleanup_on_start"`
	// BootRandomChimeOnStart picks a random chime from the configured group before presenting (TeslaUSB parity).
	BootRandomChimeOnStart bool `yaml:"boot_random_chime_on_start"`
	// BootBlockUntilReady blocks HTTP startup until the boot sequence has completed.
	BootBlockUntilReady bool `yaml:"boot_block_until_ready"`
}

type DiskImagesConfig struct {
	CamName        string `yaml:"cam_name"`
	LightshowName  string `yaml:"lightshow_name"`
	CamLabel       string `yaml:"cam_label"`
	LightshowLabel string `yaml:"lightshow_label"`
	MusicName      string `yaml:"music_name"`
	MusicLabel     string `yaml:"music_label"`
	// Part2Enabled controls whether the shared LightShow/Chimes/Wraps partition is created at all.
	// All three sub-features (chimes, lightshow, wraps) require this to be true.
	Part2Enabled     bool   `yaml:"part2_enabled"`
	ChimesEnabled    bool   `yaml:"chimes_enabled"`
	LightshowEnabled bool   `yaml:"lightshow_enabled"`
	WrapsEnabled     bool   `yaml:"wraps_enabled"`
	MusicEnabled     bool   `yaml:"music_enabled"`
	MusicFS          string `yaml:"music_fs"`
	BootFsckEnabled  bool   `yaml:"boot_fsck_enabled"`
}

type SetupConfig struct {
	Part2Size   string `yaml:"part2_size"`
	Part3Size   string `yaml:"part3_size"`
	ReserveSize string `yaml:"reserve_size"`
}

type NetworkConfig struct {
	SambaPassword string `yaml:"samba_password"`
	// SambaEnabled gates whether smbd/nmbd are managed/started by Argus.
	// Pointer so we can distinguish "unset" (defaults to enabled to preserve
	// historical behavior on upgrade) from an explicit false.
	SambaEnabled *bool `yaml:"samba_enabled,omitempty"`
	WebPort      int   `yaml:"web_port"`
}

type OfflineAPConfig struct {
	Enabled          bool   `yaml:"enabled"`
	Interface        string `yaml:"interface"`
	SSID             string `yaml:"ssid"`
	Passphrase       string `yaml:"passphrase"`
	Channel          int    `yaml:"channel"`
	IPv4CIDR         string `yaml:"ipv4_cidr"`
	DHCPStart        string `yaml:"dhcp_start"`
	DHCPEnd          string `yaml:"dhcp_end"`
	CheckInterval    int    `yaml:"check_interval"`
	DisconnectGrace  int    `yaml:"disconnect_grace"`
	MinRSSI          int    `yaml:"min_rssi"`
	StableSeconds    int    `yaml:"stable_seconds"`
	PingTarget       string `yaml:"ping_target"`
	RetrySeconds     int    `yaml:"retry_seconds"`
	VirtualInterface string `yaml:"virtual_interface"`
	ForceMode        string `yaml:"force_mode"`
	// ShareInternet routes AP clients (e.g. the car joining the Pi's hotspot)
	// out to whatever upstream the Pi currently has — Bluetooth tethering
	// (bnep0) or WiFi (wlan0) — via NAT. When false the AP stays a captive
	// admin-only portal with no internet egress.
	ShareInternet bool `yaml:"share_internet"`
}

type SystemConfig struct {
	ConfigFile           string `yaml:"config_file"`
	SambaConf            string `yaml:"samba_conf"`
	ReapplySysctlOnStart bool   `yaml:"reapply_sysctl_on_start"`
	WatchdogEnabled      bool   `yaml:"watchdog_enabled"`
	WatchdogTimeoutSec   int    `yaml:"watchdog_timeout_sec"`
}

type WebConfig struct {
	SecretKey         string  `yaml:"secret_key"`
	MaxLockChimeSize  int64   `yaml:"max_lock_chime_size"`
	MaxLockChimeDur   float64 `yaml:"max_lock_chime_duration"`
	MinLockChimeDur   float64 `yaml:"min_lock_chime_duration"`
	SpeedRangeMin     float64 `yaml:"speed_range_min"`
	SpeedRangeMax     float64 `yaml:"speed_range_max"`
	SpeedStep         float64 `yaml:"speed_step"`
	LockChimeFilename string  `yaml:"lock_chime_filename"`
	ChimesFolder      string  `yaml:"chimes_folder"`
	LightshowFolder   string  `yaml:"lightshow_folder"`
	MaxUploadSizeMB   int     `yaml:"max_upload_size_mb"`
	MaxUploadChunkMB  int     `yaml:"max_upload_chunk_mb"`

	// AuthEnabled gates the web UI/API behind a login. Pointer so an explicit
	// `auth_enabled: false` is distinguishable from unset; defaults to true.
	AuthEnabled *bool `yaml:"auth_enabled,omitempty"`
	// AuthUsername/AuthPassword are the login credentials. They ship with a
	// default (admin / argus) that should be changed on first use. Session
	// cookies are signed with SecretKey.
	AuthUsername string `yaml:"auth_username"`
	AuthPassword string `yaml:"auth_password"`
	// AllowedHosts is an optional allowlist of Host header values (e.g. a
	// reverse-proxy FQDN) accepted by the anti-DNS-rebinding check, in addition
	// to the always-allowed local identities (IPs, localhost, *.local, bare
	// single-label names).
	AllowedHosts []string `yaml:"allowed_hosts,omitempty"`
}

// DefaultAuthUsername / DefaultAuthPassword are the shipped credentials. The
// auth status endpoint reports when they are still in use so the UI can nag.
const (
	DefaultAuthUsername = "admin"
	DefaultAuthPassword = "argus"
)

type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BotToken     string `yaml:"bot_token"`
	ChatID       string `yaml:"chat_id"`
	OfflineMode  string `yaml:"offline_mode"`
	MaxQueueSize int    `yaml:"max_queue_size"`
	VideoQuality string `yaml:"video_quality"`
}

type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	// Secret is used to sign payloads with HMAC-SHA256 (X-Argus-Signature header). Optional.
	Secret string `yaml:"secret"`
}

type UpdateConfig struct {
	AutoUpdate bool `yaml:"auto_update"`
	// CheckOnStartup is a pointer so an explicit `check_on_startup: false` is
	// distinguishable from the field being unset. A plain bool defaulted to true
	// can never be turned off (unset == false == "apply default"), which silently
	// forced startup update checks on regardless of the user's config.
	CheckOnStartup *bool  `yaml:"check_on_startup,omitempty"`
	Channel        string `yaml:"channel"`
}

// ViewerPrefsConfig stores user-facing display preferences for the dashcam viewer.
// Persisted server-side so they survive app updates and re-flashes.
type ViewerPrefsConfig struct {
	SpeedUnit string  `yaml:"speed_unit"`
	HudScale  float64 `yaml:"hud_scale,omitempty"`
	MapScale  float64 `yaml:"map_scale,omitempty"`
}

const defaultSecretKey = "CHANGE-THIS-TO-A-RANDOM-SECRET-KEY-ON-FIRST-INSTALL"

var (
	globalCfg *Config
	globalMu  sync.Mutex
)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.ConfigFilePath = path

	// Detect whether setDefaults will (re)generate the web secret key. When it
	// does, persist it once here so the key stays stable across restarts;
	// otherwise every reboot rotates the HMAC key and invalidates all existing
	// login sessions (session cookies are signed with Web.SecretKey).
	persistKey := cfg.Web.SecretKey == "" || cfg.Web.SecretKey == defaultSecretKey

	cfg.setDefaults()
	cfg.computePaths()

	if persistKey {
		// Best-effort: failing to persist is non-fatal (the in-memory key still
		// works for this process), so it must not turn Load into a hard error.
		_ = cfg.Save()
	}

	return cfg, nil
}

// Get returns the global singleton config. Must call InitGlobal first.
func Get() *Config {
	globalMu.Lock()
	defer globalMu.Unlock()
	return globalCfg
}

// InitGlobal loads config and sets the global singleton.
// It is safe to call multiple times; subsequent calls replace the config under a mutex.
func InitGlobal(path string) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	globalMu.Lock()
	globalCfg = cfg
	globalMu.Unlock()
	return nil
}

func (c *Config) setDefaults() {
	if c.DiskImages.MusicName == "" {
		c.DiskImages.MusicName = "usb_music.img"
	}
	if c.DiskImages.MusicLabel == "" {
		c.DiskImages.MusicLabel = "Music"
	}
	if c.DiskImages.MusicFS == "" {
		c.DiskImages.MusicFS = "fat32"
	}
	// lightshow_enabled and wraps_enabled default to true when not explicitly set to false.
	// Since YAML omits false booleans, we can't distinguish "not set" from "false" without
	// a pointer. Instead, the config template always writes them explicitly (true by default).
	// setDefaults does NOT override explicitly-set false values here.
	if c.Network.WebPort == 0 {
		c.Network.WebPort = 80
	}
	if c.Web.MaxUploadSizeMB == 0 {
		c.Web.MaxUploadSizeMB = 2048
	}
	if c.Web.MaxUploadChunkMB == 0 {
		c.Web.MaxUploadChunkMB = 16
	}
	if c.Web.LockChimeFilename == "" {
		c.Web.LockChimeFilename = "LockChime.wav"
	}
	if c.Web.ChimesFolder == "" {
		c.Web.ChimesFolder = "Chimes"
	}
	if c.Web.LightshowFolder == "" {
		c.Web.LightshowFolder = "LightShow"
	}
	if c.Web.SecretKey == "" || c.Web.SecretKey == defaultSecretKey {
		c.Web.SecretKey = generateRandomKey()
	}
	if c.Web.AuthEnabled == nil {
		enabled := true
		c.Web.AuthEnabled = &enabled
	}
	if c.Web.AuthUsername == "" {
		c.Web.AuthUsername = DefaultAuthUsername
	}
	if c.Web.AuthPassword == "" {
		c.Web.AuthPassword = DefaultAuthPassword
	}
	if c.Telegram.OfflineMode == "" {
		c.Telegram.OfflineMode = "queue"
	}
	if c.Telegram.MaxQueueSize == 0 {
		c.Telegram.MaxQueueSize = 50
	}
	if c.Telegram.VideoQuality == "" {
		c.Telegram.VideoQuality = "hd"
	}
	if c.OfflineAP.ForceMode == "" {
		c.OfflineAP.ForceMode = "auto"
	}
	if c.OfflineAP.VirtualInterface == "" {
		c.OfflineAP.VirtualInterface = "uap0"
	}
	if c.OfflineAP.PingTarget == "" {
		c.OfflineAP.PingTarget = "8.8.8.8"
	}
	if c.OfflineAP.CheckInterval == 0 {
		c.OfflineAP.CheckInterval = 30
	}
	if c.OfflineAP.DisconnectGrace == 0 {
		c.OfflineAP.DisconnectGrace = 60
	}
	if c.Update.CheckOnStartup == nil {
		enabled := true
		c.Update.CheckOnStartup = &enabled
	}
	if c.Update.Channel == "" {
		c.Update.Channel = "stable"
	}
	if c.LogLevel == "" {
		c.LogLevel = "debug"
	}
	if c.System.WatchdogTimeoutSec <= 0 {
		// 60s matches TeslaUSB/mphacker guidance for large images; the daemon clamps to hardware max.
		c.System.WatchdogTimeoutSec = 60
	} else if c.System.WatchdogTimeoutSec < 10 {
		c.System.WatchdogTimeoutSec = 10
	}
	if c.ViewerPrefs.SpeedUnit == "" {
		c.ViewerPrefs.SpeedUnit = "kph"
	}
	if c.ViewerPrefs.HudScale <= 0 {
		c.ViewerPrefs.HudScale = 1.0
	}
	if c.ViewerPrefs.MapScale <= 0 {
		c.ViewerPrefs.MapScale = 1.0
	}
	if c.Network.SambaEnabled == nil {
		enabled := true
		c.Network.SambaEnabled = &enabled
	}
}

// SambaEnabled reports whether Argus should run/manage Samba.
// Defaults to true when the field is unset to preserve existing installs.
func (c *Config) SambaEnabled() bool {
	return c.Network.SambaEnabled == nil || *c.Network.SambaEnabled
}

// SetSambaEnabled records the Samba enabled flag.
func (c *Config) SetSambaEnabled(enabled bool) {
	c.Network.SambaEnabled = &enabled
}

// CheckUpdateOnStartup reports whether Argus should check for updates at boot.
// Defaults to true when the field is unset to preserve existing behavior.
func (c *Config) CheckUpdateOnStartup() bool {
	return c.Update.CheckOnStartup == nil || *c.Update.CheckOnStartup
}

// AuthEnabled reports whether the web UI/API requires a login.
// Defaults to true when unset.
func (c *Config) AuthEnabled() bool {
	return c.Web.AuthEnabled == nil || *c.Web.AuthEnabled
}

// UsingDefaultAuth reports whether the shipped default credentials are still in
// use, so the UI can prompt the operator to change them.
func (c *Config) UsingDefaultAuth() bool {
	return c.Web.AuthUsername == DefaultAuthUsername && c.Web.AuthPassword == DefaultAuthPassword
}

func (c *Config) computePaths() {
	c.GadgetDir = filepath.Dir(c.ConfigFilePath)
	c.MountDir = c.Installation.MountDir
	c.ImgCamPath = filepath.Join(c.GadgetDir, c.DiskImages.CamName)
	c.ImgLightshow = filepath.Join(c.GadgetDir, c.DiskImages.LightshowName)
	c.ImgMusicPath = filepath.Join(c.GadgetDir, c.DiskImages.MusicName)
	c.StateFile = filepath.Join(c.GadgetDir, "state.txt")
	c.ThumbnailDir = filepath.Join(c.GadgetDir, "thumbnails")
}

// USBPartitions returns the active partition identifiers based on enabled features.
func (c *Config) USBPartitions() []string {
	parts := []string{"part1"}
	if c.DiskImages.Part2Enabled {
		parts = append(parts, "part2")
	}
	if c.DiskImages.MusicEnabled {
		parts = append(parts, "part3")
	}
	return parts
}

// MountPath returns the mount path for a partition in the given mode.
func (c *Config) MountPath(partition string, readOnly bool) string {
	suffix := ""
	if readOnly {
		suffix = "-ro"
	}
	return filepath.Join(c.MountDir, partition+suffix)
}

// CameraAngles returns the six Tesla camera identifiers.
func (c *Config) CameraAngles() []string {
	return []string{
		"front", "back",
		"left_repeater", "right_repeater",
		"left_pillar", "right_pillar",
	}
}

// Save writes the current config back to its YAML file atomically.
// Calls are serialized so concurrent savers can't clobber the shared temp file.
func (c *Config) Save() error {
	c.saveMu.Lock()
	defer c.saveMu.Unlock()

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := c.ConfigFilePath + ".tmp"
	// Config contains secrets (Samba password, Telegram bot token, webhook
	// secret, web secret key) — restrict to the owner.
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write config temp: %w", err)
	}
	if err := os.Rename(tmp, c.ConfigFilePath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

func generateRandomKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "fallback-insecure-key-please-change"
	}
	return hex.EncodeToString(b)
}
