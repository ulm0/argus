package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/system/watchdog"
)

// ConfigHandler exposes config read and partial-update endpoints.
// Storage-related fields (disk image names, partition flags, setup sizes) are
// read-only because they are one-time-setup operations.
type ConfigHandler struct {
	cfg *config.Config
}

func NewConfigHandler(cfg *config.Config) *ConfigHandler {
	return &ConfigHandler{cfg: cfg}
}

// configResponse is the public shape returned by GET /api/config.
// Read-only sections are nested under dedicated keys to make the boundary
// explicit in the API contract.
type configResponse struct {
	// Editable fields
	Network     networkConfigPublic     `json:"network"`
	OfflineAP   offlineAPPublic         `json:"offline_ap"`
	Web         webConfigPublic         `json:"web"`
	Telegram    telegramConfigPublic    `json:"telegram"`
	Update      updateConfigPublic      `json:"update"`
	Startup     startupConfigPublic     `json:"startup"`
	ViewerPrefs viewerPrefsConfigPublic `json:"viewer_prefs"`
	Auth        authConfigPublic        `json:"auth"`
	Archive     archiveConfigPublic     `json:"archive"`
	LogLevel    string                  `json:"log_level"`

	// Read-only info (not patchable)
	Storage storageInfo `json:"storage"`
}

type networkConfigPublic struct {
	WebPort int `json:"web_port"`
}

// offlineAPPublic deliberately omits the AP passphrase: it is write-only here,
// and the Show/Copy UI reads it from the AP status endpoint instead.
type offlineAPPublic struct {
	Enabled         bool   `json:"enabled"`
	SSID            string `json:"ssid"`
	Channel         int    `json:"channel"`
	IPv4CIDR        string `json:"ipv4_cidr"`
	DHCPStart       string `json:"dhcp_start"`
	DHCPEnd         string `json:"dhcp_end"`
	CheckInterval   int    `json:"check_interval"`
	DisconnectGrace int    `json:"disconnect_grace"`
	MinRSSI         int    `json:"min_rssi"`
	StableSeconds   int    `json:"stable_seconds"`
	PingTarget      string `json:"ping_target"`
	RetrySeconds    int    `json:"retry_seconds"`
	ForceMode       string `json:"force_mode"`
}

type webConfigPublic struct {
	MaxLockChimeSize int64   `json:"max_lock_chime_size"`
	MaxLockChimeDur  float64 `json:"max_lock_chime_duration"`
	MinLockChimeDur  float64 `json:"min_lock_chime_duration"`
	SpeedRangeMin    float64 `json:"speed_range_min"`
	SpeedRangeMax    float64 `json:"speed_range_max"`
	SpeedStep        float64 `json:"speed_step"`
	MaxUploadSizeMB  int     `json:"max_upload_size_mb"`
	MaxUploadChunkMB int     `json:"max_upload_chunk_mb"`
}

type telegramConfigPublic struct {
	Enabled bool `json:"enabled"`
	// The token itself is never returned; the UI only needs to know one is set.
	BotTokenSet  bool   `json:"bot_token_set"`
	ChatID       string `json:"chat_id"`
	OfflineMode  string `json:"offline_mode"`
	MaxQueueSize int    `json:"max_queue_size"`
	VideoQuality string `json:"video_quality"`
}

type updateConfigPublic struct {
	AutoUpdate     bool   `json:"auto_update"`
	CheckOnStartup bool   `json:"check_on_startup"`
	Channel        string `json:"channel"`
}

// authConfigPublic exposes login settings. The password is write-only and is
// never returned; using_default lets the UI nag to change the shipped creds.
type authConfigPublic struct {
	Enabled      bool   `json:"enabled"`
	Username     string `json:"username"`
	UsingDefault bool   `json:"using_default"`
}

type viewerPrefsConfigPublic struct {
	SpeedUnit string  `json:"speed_unit"`
	HudScale  float64 `json:"hud_scale"`
	MapScale  float64 `json:"map_scale"`
}

type startupConfigPublic struct {
	BootPresentOnStart     bool `json:"boot_present_on_start"`
	BootBlockUntilReady    bool `json:"boot_block_until_ready"`
	BootCleanupOnStart     bool `json:"boot_cleanup_on_start"`
	BootRandomChimeOnStart bool `json:"boot_random_chime_on_start"`
	BootFsckEnabled        bool `json:"boot_fsck_enabled"`
	WatchdogEnabled        bool `json:"watchdog_enabled"`
	WatchdogTimeoutSec     int  `json:"watchdog_timeout_sec"`
	ReapplySysctlOnStart   bool `json:"reapply_sysctl_on_start"`
}

type archiveConfigPublic struct {
	Enabled       bool   `json:"enabled"`
	SSID          string `json:"ssid"`
	TargetPath    string `json:"target_path"`
	IncludeRecent bool   `json:"include_recent"`
}

type storageInfo struct {
	CamName          string `json:"cam_name"`
	CamLabel         string `json:"cam_label"`
	Part2Enabled     bool   `json:"part2_enabled"`
	ChimesEnabled    bool   `json:"chimes_enabled"`
	LightshowEnabled bool   `json:"lightshow_enabled"`
	WrapsEnabled     bool   `json:"wraps_enabled"`
	MusicEnabled     bool   `json:"music_enabled"`
	MusicFS          string `json:"music_fs"`
	BootFsckEnabled  bool   `json:"boot_fsck_enabled"`
	InstallDir       string `json:"install_dir"`
	MountDir         string `json:"mount_dir"`
	TargetUser       string `json:"target_user"`
}

// Get returns the current configuration, separating editable from read-only fields.
func (h *ConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg
	resp := configResponse{
		Network: networkConfigPublic{
			WebPort: cfg.Network.WebPort,
		},
		OfflineAP: offlineAPPublic{
			Enabled:         cfg.OfflineAP.Enabled,
			SSID:            cfg.OfflineAP.SSID,
			Channel:         cfg.OfflineAP.Channel,
			IPv4CIDR:        cfg.OfflineAP.IPv4CIDR,
			DHCPStart:       cfg.OfflineAP.DHCPStart,
			DHCPEnd:         cfg.OfflineAP.DHCPEnd,
			CheckInterval:   cfg.OfflineAP.CheckInterval,
			DisconnectGrace: cfg.OfflineAP.DisconnectGrace,
			MinRSSI:         cfg.OfflineAP.MinRSSI,
			StableSeconds:   cfg.OfflineAP.StableSeconds,
			PingTarget:      cfg.OfflineAP.PingTarget,
			RetrySeconds:    cfg.OfflineAP.RetrySeconds,
			ForceMode:       cfg.OfflineAP.ForceMode,
		},
		Web: webConfigPublic{
			MaxLockChimeSize: cfg.Web.MaxLockChimeSize,
			MaxLockChimeDur:  cfg.Web.MaxLockChimeDur,
			MinLockChimeDur:  cfg.Web.MinLockChimeDur,
			SpeedRangeMin:    cfg.Web.SpeedRangeMin,
			SpeedRangeMax:    cfg.Web.SpeedRangeMax,
			SpeedStep:        cfg.Web.SpeedStep,
			MaxUploadSizeMB:  cfg.Web.MaxUploadSizeMB,
			MaxUploadChunkMB: cfg.Web.MaxUploadChunkMB,
		},
		Telegram: telegramConfigPublic{
			Enabled:      cfg.Telegram.Enabled,
			BotTokenSet:  cfg.Telegram.BotToken != "",
			ChatID:       cfg.Telegram.ChatID,
			OfflineMode:  cfg.Telegram.OfflineMode,
			MaxQueueSize: cfg.Telegram.MaxQueueSize,
			VideoQuality: cfg.Telegram.VideoQuality,
		},
		Update: updateConfigPublic{
			AutoUpdate:     cfg.Update.AutoUpdate,
			CheckOnStartup: cfg.CheckUpdateOnStartup(),
			Channel:        cfg.Update.Channel,
		},
		Startup: startupConfigPublic{
			BootPresentOnStart:     cfg.Installation.BootPresentOnStart,
			BootBlockUntilReady:    cfg.Installation.BootBlockUntilReady,
			BootCleanupOnStart:     cfg.Installation.BootCleanupOnStart,
			BootRandomChimeOnStart: cfg.Installation.BootRandomChimeOnStart,
			BootFsckEnabled:        cfg.DiskImages.BootFsckEnabled,
			WatchdogEnabled:        cfg.System.WatchdogEnabled,
			WatchdogTimeoutSec:     cfg.System.WatchdogTimeoutSec,
			ReapplySysctlOnStart:   cfg.System.ReapplySysctlOnStart,
		},
		ViewerPrefs: viewerPrefsConfigPublic{
			SpeedUnit: cfg.ViewerPrefs.SpeedUnit,
			HudScale:  cfg.ViewerPrefs.HudScale,
			MapScale:  cfg.ViewerPrefs.MapScale,
		},
		Auth: authConfigPublic{
			Enabled:      cfg.AuthEnabled(),
			Username:     cfg.Web.AuthUsername,
			UsingDefault: cfg.UsingDefaultAuth(),
		},
		Archive: archiveConfigPublic{
			Enabled:       cfg.Archive.Enabled,
			SSID:          cfg.Archive.SSID,
			TargetPath:    cfg.Archive.TargetPath,
			IncludeRecent: cfg.Archive.IncludeRecent,
		},
		LogLevel: cfg.LogLevel,
		Storage: storageInfo{
			CamName:          cfg.DiskImages.CamName,
			CamLabel:         cfg.DiskImages.CamLabel,
			Part2Enabled:     cfg.DiskImages.Part2Enabled,
			ChimesEnabled:    cfg.DiskImages.ChimesEnabled,
			LightshowEnabled: cfg.DiskImages.LightshowEnabled,
			WrapsEnabled:     cfg.DiskImages.WrapsEnabled,
			MusicEnabled:     cfg.DiskImages.MusicEnabled,
			MusicFS:          cfg.DiskImages.MusicFS,
			BootFsckEnabled:  cfg.DiskImages.BootFsckEnabled,
			InstallDir:       cfg.GadgetDir,
			MountDir:         cfg.Installation.MountDir,
			TargetUser:       cfg.Installation.TargetUser,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// patchRequest contains only the fields that are allowed to be updated.
type patchRequest struct {
	Network     *networkPatch     `json:"network,omitempty"`
	OfflineAP   *offlineAPPatch   `json:"offline_ap,omitempty"`
	Web         *webPatch         `json:"web,omitempty"`
	Telegram    *telegramPatch    `json:"telegram,omitempty"`
	Update      *updatePatch      `json:"update,omitempty"`
	Startup     *startupPatch     `json:"startup,omitempty"`
	ViewerPrefs *viewerPrefsPatch `json:"viewer_prefs,omitempty"`
	Auth        *authPatch        `json:"auth,omitempty"`
	Archive     *archivePatch     `json:"archive,omitempty"`
	LogLevel    *string           `json:"log_level,omitempty"`
}

type archivePatch struct {
	Enabled       *bool   `json:"enabled,omitempty"`
	SSID          *string `json:"ssid,omitempty"`
	TargetPath    *string `json:"target_path,omitempty"`
	IncludeRecent *bool   `json:"include_recent,omitempty"`
}

type authPatch struct {
	Enabled  *bool   `json:"enabled,omitempty"`
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
}

type networkPatch struct {
	WebPort *int `json:"web_port,omitempty"`
}

type offlineAPPatch struct {
	Enabled         *bool   `json:"enabled,omitempty"`
	SSID            *string `json:"ssid,omitempty"`
	Passphrase      *string `json:"passphrase,omitempty"`
	Channel         *int    `json:"channel,omitempty"`
	IPv4CIDR        *string `json:"ipv4_cidr,omitempty"`
	DHCPStart       *string `json:"dhcp_start,omitempty"`
	DHCPEnd         *string `json:"dhcp_end,omitempty"`
	CheckInterval   *int    `json:"check_interval,omitempty"`
	DisconnectGrace *int    `json:"disconnect_grace,omitempty"`
	MinRSSI         *int    `json:"min_rssi,omitempty"`
	StableSeconds   *int    `json:"stable_seconds,omitempty"`
	PingTarget      *string `json:"ping_target,omitempty"`
	RetrySeconds    *int    `json:"retry_seconds,omitempty"`
	ForceMode       *string `json:"force_mode,omitempty"`
}

type webPatch struct {
	MaxLockChimeSize *int64   `json:"max_lock_chime_size,omitempty"`
	MaxLockChimeDur  *float64 `json:"max_lock_chime_duration,omitempty"`
	MinLockChimeDur  *float64 `json:"min_lock_chime_duration,omitempty"`
	SpeedRangeMin    *float64 `json:"speed_range_min,omitempty"`
	SpeedRangeMax    *float64 `json:"speed_range_max,omitempty"`
	SpeedStep        *float64 `json:"speed_step,omitempty"`
	MaxUploadSizeMB  *int     `json:"max_upload_size_mb,omitempty"`
	MaxUploadChunkMB *int     `json:"max_upload_chunk_mb,omitempty"`
}

type telegramPatch struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	BotToken     *string `json:"bot_token,omitempty"`
	ChatID       *string `json:"chat_id,omitempty"`
	OfflineMode  *string `json:"offline_mode,omitempty"`
	MaxQueueSize *int    `json:"max_queue_size,omitempty"`
	VideoQuality *string `json:"video_quality,omitempty"`
}

type updatePatch struct {
	AutoUpdate     *bool   `json:"auto_update,omitempty"`
	CheckOnStartup *bool   `json:"check_on_startup,omitempty"`
	Channel        *string `json:"channel,omitempty"`
}

type viewerPrefsPatch struct {
	SpeedUnit *string  `json:"speed_unit,omitempty"`
	HudScale  *float64 `json:"hud_scale,omitempty"`
	MapScale  *float64 `json:"map_scale,omitempty"`
}

type startupPatch struct {
	BootPresentOnStart     *bool `json:"boot_present_on_start,omitempty"`
	BootBlockUntilReady    *bool `json:"boot_block_until_ready,omitempty"`
	BootCleanupOnStart     *bool `json:"boot_cleanup_on_start,omitempty"`
	BootRandomChimeOnStart *bool `json:"boot_random_chime_on_start,omitempty"`
	BootFsckEnabled        *bool `json:"boot_fsck_enabled,omitempty"`
	WatchdogEnabled        *bool `json:"watchdog_enabled,omitempty"`
	WatchdogTimeoutSec     *int  `json:"watchdog_timeout_sec,omitempty"`
	ReapplySysctlOnStart   *bool `json:"reapply_sysctl_on_start,omitempty"`
}

// Patch applies a partial update to the mutable config sections and persists to disk.
func (h *ConfigHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var req patchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate every section BEFORE mutating anything. h.cfg is the shared,
	// process-wide config pointer, so applying section-by-section and only then
	// rejecting a later section would leave the live config partially changed
	// (and diverged from disk, since Save never runs on the error path).
	if msg := validatePatch(&req); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}

	cfg := h.cfg

	if p := req.Network; p != nil {
		if p.WebPort != nil {
			cfg.Network.WebPort = *p.WebPort
		}
	}

	if p := req.OfflineAP; p != nil {
		// Network fields (SSID/passphrase/CIDR/DHCP/ping target) were validated
		// up front by validatePatch so nothing malformed reaches hostapd/dnsmasq/ip.
		if p.Enabled != nil {
			cfg.OfflineAP.Enabled = *p.Enabled
		}
		if p.SSID != nil {
			cfg.OfflineAP.SSID = *p.SSID
		}
		if p.Passphrase != nil {
			cfg.OfflineAP.Passphrase = *p.Passphrase
		}
		if p.Channel != nil {
			cfg.OfflineAP.Channel = *p.Channel
		}
		if p.IPv4CIDR != nil {
			cfg.OfflineAP.IPv4CIDR = *p.IPv4CIDR
		}
		if p.DHCPStart != nil {
			cfg.OfflineAP.DHCPStart = *p.DHCPStart
		}
		if p.DHCPEnd != nil {
			cfg.OfflineAP.DHCPEnd = *p.DHCPEnd
		}
		if p.CheckInterval != nil {
			cfg.OfflineAP.CheckInterval = *p.CheckInterval
		}
		if p.DisconnectGrace != nil {
			cfg.OfflineAP.DisconnectGrace = *p.DisconnectGrace
		}
		if p.MinRSSI != nil {
			cfg.OfflineAP.MinRSSI = *p.MinRSSI
		}
		if p.StableSeconds != nil {
			cfg.OfflineAP.StableSeconds = *p.StableSeconds
		}
		if p.PingTarget != nil {
			cfg.OfflineAP.PingTarget = *p.PingTarget
		}
		if p.RetrySeconds != nil {
			cfg.OfflineAP.RetrySeconds = *p.RetrySeconds
		}
		if p.ForceMode != nil {
			cfg.OfflineAP.ForceMode = *p.ForceMode
		}
	}

	if p := req.Web; p != nil {
		if p.MaxLockChimeSize != nil {
			cfg.Web.MaxLockChimeSize = *p.MaxLockChimeSize
		}
		if p.MaxLockChimeDur != nil {
			cfg.Web.MaxLockChimeDur = *p.MaxLockChimeDur
		}
		if p.MinLockChimeDur != nil {
			cfg.Web.MinLockChimeDur = *p.MinLockChimeDur
		}
		if p.SpeedRangeMin != nil {
			cfg.Web.SpeedRangeMin = *p.SpeedRangeMin
		}
		if p.SpeedRangeMax != nil {
			cfg.Web.SpeedRangeMax = *p.SpeedRangeMax
		}
		if p.SpeedStep != nil {
			cfg.Web.SpeedStep = *p.SpeedStep
		}
		if p.MaxUploadSizeMB != nil {
			cfg.Web.MaxUploadSizeMB = *p.MaxUploadSizeMB
		}
		if p.MaxUploadChunkMB != nil {
			cfg.Web.MaxUploadChunkMB = *p.MaxUploadChunkMB
		}
	}

	if p := req.Telegram; p != nil {
		if p.Enabled != nil {
			cfg.Telegram.Enabled = *p.Enabled
		}
		if p.BotToken != nil {
			cfg.Telegram.BotToken = *p.BotToken
		}
		if p.ChatID != nil {
			cfg.Telegram.ChatID = *p.ChatID
		}
		if p.OfflineMode != nil {
			cfg.Telegram.OfflineMode = *p.OfflineMode
		}
		if p.MaxQueueSize != nil {
			cfg.Telegram.MaxQueueSize = *p.MaxQueueSize
		}
		if p.VideoQuality != nil {
			cfg.Telegram.VideoQuality = *p.VideoQuality
		}
	}

	if p := req.Update; p != nil {
		if p.AutoUpdate != nil {
			cfg.Update.AutoUpdate = *p.AutoUpdate
		}
		if p.CheckOnStartup != nil {
			v := *p.CheckOnStartup
			cfg.Update.CheckOnStartup = &v
		}
		if p.Channel != nil {
			cfg.Update.Channel = *p.Channel
		}
	}

	if p := req.Startup; p != nil {
		if p.BootPresentOnStart != nil {
			cfg.Installation.BootPresentOnStart = *p.BootPresentOnStart
		}
		if p.BootBlockUntilReady != nil {
			cfg.Installation.BootBlockUntilReady = *p.BootBlockUntilReady
		}
		if p.BootCleanupOnStart != nil {
			cfg.Installation.BootCleanupOnStart = *p.BootCleanupOnStart
		}
		if p.BootRandomChimeOnStart != nil {
			cfg.Installation.BootRandomChimeOnStart = *p.BootRandomChimeOnStart
		}
		if p.BootFsckEnabled != nil {
			cfg.DiskImages.BootFsckEnabled = *p.BootFsckEnabled
		}
		if p.WatchdogEnabled != nil {
			cfg.System.WatchdogEnabled = *p.WatchdogEnabled
		}
		if p.ReapplySysctlOnStart != nil {
			cfg.System.ReapplySysctlOnStart = *p.ReapplySysctlOnStart
		}
		if p.WatchdogTimeoutSec != nil {
			cfg.System.WatchdogTimeoutSec = *p.WatchdogTimeoutSec
		}
	}

	if p := req.ViewerPrefs; p != nil {
		if p.SpeedUnit != nil {
			cfg.ViewerPrefs.SpeedUnit = strings.TrimSpace(strings.ToLower(*p.SpeedUnit))
		}
		if p.HudScale != nil {
			cfg.ViewerPrefs.HudScale = *p.HudScale
		}
		if p.MapScale != nil {
			cfg.ViewerPrefs.MapScale = *p.MapScale
		}
	}

	if p := req.Auth; p != nil {
		if p.Username != nil {
			cfg.Web.AuthUsername = strings.TrimSpace(*p.Username)
		}
		if p.Password != nil {
			cfg.Web.AuthPassword = *p.Password
		}
		if p.Enabled != nil {
			cfg.Web.AuthEnabled = p.Enabled
		}
	}

	if p := req.Archive; p != nil {
		if p.Enabled != nil {
			cfg.Archive.Enabled = *p.Enabled
		}
		if p.SSID != nil {
			cfg.Archive.SSID = *p.SSID
		}
		if p.TargetPath != nil {
			cfg.Archive.TargetPath = strings.TrimSpace(*p.TargetPath)
		}
		if p.IncludeRecent != nil {
			cfg.Archive.IncludeRecent = *p.IncludeRecent
		}
	}

	if req.LogLevel != nil {
		lvl := strings.TrimSpace(strings.ToLower(*req.LogLevel))
		logger.SetLevelFromString(lvl)
		cfg.LogLevel = lvl
	}

	if err := cfg.Save(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	if req.Startup != nil && (req.Startup.WatchdogEnabled != nil || req.Startup.WatchdogTimeoutSec != nil) {
		if err := watchdog.ApplyDaemon(cfg.System.WatchdogEnabled, cfg.System.WatchdogTimeoutSec); err != nil {
			logger.L.WithError(err).Warn("watchdog daemon apply failed")
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// validatePatch checks every mutable section of a patch request without touching
// any config, returning a non-empty error message on the first violation. This
// lets Patch reject bad input before applying any change, so a failure in a late
// section can't leave earlier sections half-applied to the live config.
func validatePatch(req *patchRequest) string {
	// Out-of-range web_port survives Load (setDefaults only re-defaults 0->80) and
	// bricks ListenAndServe on the next boot, leaving the headless UI unreachable.
	if p := req.Network; p != nil && p.WebPort != nil && (*p.WebPort < 1 || *p.WebPort > 65535) {
		return "web_port must be in [1, 65535]"
	}

	if p := req.OfflineAP; p != nil {
		// Same checks UpdateAPConfig enforces, so a PATCH can't smuggle
		// newline-injected SSID/passphrase, non-IP DHCP/CIDR, or a leading-'-'
		// ping target into hostapd/dnsmasq/ip.
		if p.SSID != nil && (len(*p.SSID) > 32 || strings.ContainsAny(*p.SSID, "\r\n")) {
			return "invalid ssid"
		}
		if p.Passphrase != nil {
			if l := len(*p.Passphrase); l < 8 || l > 63 || strings.ContainsAny(*p.Passphrase, "\r\n") {
				return "invalid passphrase (8-63 chars, no newlines)"
			}
		}
		if p.IPv4CIDR != nil {
			if _, _, err := net.ParseCIDR(*p.IPv4CIDR); err != nil {
				return "invalid ipv4_cidr"
			}
		}
		if p.DHCPStart != nil && net.ParseIP(*p.DHCPStart) == nil {
			return "invalid dhcp_start"
		}
		if p.DHCPEnd != nil && net.ParseIP(*p.DHCPEnd) == nil {
			return "invalid dhcp_end"
		}
		if p.PingTarget != nil && (*p.PingTarget == "" || strings.HasPrefix(*p.PingTarget, "-")) {
			return "invalid ping_target"
		}
	}

	if p := req.Web; p != nil {
		// The UI's number inputs yield Number("") === 0 when cleared, so a zero or
		// negative limit here is a real request that would break uploads and chime
		// validation device-wide until someone edits the config by hand.
		if p.MaxLockChimeSize != nil && *p.MaxLockChimeSize <= 0 {
			return "max_lock_chime_size must be > 0"
		}
		if p.MaxLockChimeDur != nil && *p.MaxLockChimeDur <= 0 {
			return "max_lock_chime_duration must be > 0"
		}
		if p.MinLockChimeDur != nil && *p.MinLockChimeDur <= 0 {
			return "min_lock_chime_duration must be > 0"
		}
		if p.SpeedStep != nil && *p.SpeedStep <= 0 {
			return "speed_step must be > 0"
		}
		if p.MaxUploadSizeMB != nil && *p.MaxUploadSizeMB <= 0 {
			return "max_upload_size_mb must be > 0"
		}
		if p.MaxUploadChunkMB != nil && *p.MaxUploadChunkMB <= 0 {
			return "max_upload_chunk_mb must be > 0"
		}
		if p.MinLockChimeDur != nil && p.MaxLockChimeDur != nil && *p.MinLockChimeDur > *p.MaxLockChimeDur {
			return "min_lock_chime_duration must be <= max_lock_chime_duration"
		}
		if p.SpeedRangeMin != nil && p.SpeedRangeMax != nil && *p.SpeedRangeMin >= *p.SpeedRangeMax {
			return "speed_range_min must be < speed_range_max"
		}
		if p.MaxUploadChunkMB != nil && p.MaxUploadSizeMB != nil && *p.MaxUploadChunkMB > *p.MaxUploadSizeMB {
			return "max_upload_chunk_mb must be <= max_upload_size_mb"
		}
	}

	if p := req.Archive; p != nil {
		// The archive service rsyncs into this path; a relative one would resolve
		// against the service's working directory instead of the mounted target.
		if p.TargetPath != nil {
			path := strings.TrimSpace(*p.TargetPath)
			if path != "" && !filepath.IsAbs(path) {
				return "archive target_path must be an absolute path"
			}
			if path == "" && p.Enabled != nil && *p.Enabled {
				return "archive target_path is required when archive is enabled"
			}
		}
	}

	// Match Debian watchdog(8) / TeslaUSB-style minimum (low values risk timing issues on Pi).
	if p := req.Startup; p != nil && p.WatchdogTimeoutSec != nil && *p.WatchdogTimeoutSec < 10 {
		return "watchdog_timeout_sec must be >= 10"
	}

	if p := req.ViewerPrefs; p != nil {
		if p.SpeedUnit != nil {
			unit := strings.TrimSpace(strings.ToLower(*p.SpeedUnit))
			if unit != "kph" && unit != "mph" {
				return "invalid speed_unit (must be 'kph' or 'mph')"
			}
		}
		// HUD/Map scale bounds mirror the limits enforced in the web client
		// (see DashcamPlayer / ArgusHUD), with a slightly wider envelope so
		// future client tweaks don't require a backend update.
		if p.HudScale != nil && (*p.HudScale < 0.5 || *p.HudScale > 2.5) {
			return "hud_scale must be in [0.5, 2.5]"
		}
		if p.MapScale != nil && (*p.MapScale < 0.4 || *p.MapScale > 3.0) {
			return "map_scale must be in [0.4, 3.0]"
		}
	}

	if p := req.Auth; p != nil {
		if p.Username != nil && strings.TrimSpace(*p.Username) == "" {
			return "username must not be empty"
		}
		if p.Password != nil && len(*p.Password) < 4 {
			return "password must be at least 4 characters"
		}
	}

	if req.LogLevel != nil && !logger.ValidLevel(*req.LogLevel) {
		return "invalid log_level: " + *req.LogLevel
	}

	return ""
}
