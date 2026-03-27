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
	Update       UpdateConfig       `yaml:"update"`
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
}

type InstallationConfig struct {
	TargetUser string `yaml:"target_user"`
	MountDir   string `yaml:"mount_dir"`
	// BootPresentOnStart runs SwitchToPresent once at process start (replaces TeslaUSB present_usb_on_boot.service).
	BootPresentOnStart bool `yaml:"boot_present_on_start"`
	// BootCleanupOnStart runs cleanup against TeslaCam before presenting when cleanup_config.json has boot_cleanup policies.
	BootCleanupOnStart bool `yaml:"boot_cleanup_on_start"`
	// BootRandomChimeOnStart picks a random chime from the configured group before presenting (TeslaUSB parity).
	BootRandomChimeOnStart bool `yaml:"boot_random_chime_on_start"`
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
	Part2Enabled    bool `yaml:"part2_enabled"`
	ChimesEnabled   bool `yaml:"chimes_enabled"`
	LightshowEnabled bool `yaml:"lightshow_enabled"`
	WrapsEnabled    bool `yaml:"wraps_enabled"`
	MusicEnabled    bool `yaml:"music_enabled"`
	MusicFS         string `yaml:"music_fs"`
	BootFsckEnabled bool   `yaml:"boot_fsck_enabled"`
}

type SetupConfig struct {
	Part2Size   string `yaml:"part2_size"`
	Part3Size   string `yaml:"part3_size"`
	ReserveSize string `yaml:"reserve_size"`
}

type NetworkConfig struct {
	SambaPassword string `yaml:"samba_password"`
	WebPort       int    `yaml:"web_port"`
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
}

type SystemConfig struct {
	ConfigFile string `yaml:"config_file"`
	SambaConf  string `yaml:"samba_conf"`
}

type WebConfig struct {
	SecretKey           string  `yaml:"secret_key"`
	MaxLockChimeSize    int64   `yaml:"max_lock_chime_size"`
	MaxLockChimeDur     float64 `yaml:"max_lock_chime_duration"`
	MinLockChimeDur     float64 `yaml:"min_lock_chime_duration"`
	SpeedRangeMin       float64 `yaml:"speed_range_min"`
	SpeedRangeMax       float64 `yaml:"speed_range_max"`
	SpeedStep           float64 `yaml:"speed_step"`
	LockChimeFilename   string  `yaml:"lock_chime_filename"`
	ChimesFolder        string  `yaml:"chimes_folder"`
	LightshowFolder     string  `yaml:"lightshow_folder"`
	MaxUploadSizeMB     int     `yaml:"max_upload_size_mb"`
	MaxUploadChunkMB    int     `yaml:"max_upload_chunk_mb"`
}

type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BotToken     string `yaml:"bot_token"`
	ChatID       string `yaml:"chat_id"`
	OfflineMode  string `yaml:"offline_mode"`
	MaxQueueSize int    `yaml:"max_queue_size"`
	VideoQuality string `yaml:"video_quality"`
}

type UpdateConfig struct {
	AutoUpdate     bool   `yaml:"auto_update"`
	CheckOnStartup bool   `yaml:"check_on_startup"`
	Channel        string `yaml:"channel"`
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
	cfg.setDefaults()
	cfg.computePaths()

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
	if !c.Update.CheckOnStartup {
		c.Update.CheckOnStartup = true
	}
	if c.Update.Channel == "" {
		c.Update.Channel = "stable"
	}
	if c.LogLevel == "" {
		c.LogLevel = "debug"
	}
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
func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := c.ConfigFilePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
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
