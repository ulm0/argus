package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/ulm0/argus/internal/config"
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
	Network  networkConfigPublic  `json:"network"`
	OfflineAP offlineAPPublic     `json:"offline_ap"`
	Web      webConfigPublic      `json:"web"`
	Telegram telegramConfigPublic `json:"telegram"`
	Update   updateConfigPublic   `json:"update"`

	// Read-only info (not patchable)
	Storage storageInfo `json:"storage"`
}

type networkConfigPublic struct {
	WebPort int `json:"web_port"`
}

type offlineAPPublic struct {
	Enabled         bool   `json:"enabled"`
	SSID            string `json:"ssid"`
	Passphrase      string `json:"passphrase"`
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
	Enabled      bool   `json:"enabled"`
	BotToken     string `json:"bot_token"`
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

type storageInfo struct {
	CamName        string `json:"cam_name"`
	CamLabel       string `json:"cam_label"`
	Part2Enabled   bool   `json:"part2_enabled"`
	ChimesEnabled  bool   `json:"chimes_enabled"`
	LightshowEnabled bool `json:"lightshow_enabled"`
	WrapsEnabled   bool   `json:"wraps_enabled"`
	MusicEnabled   bool   `json:"music_enabled"`
	MusicFS        string `json:"music_fs"`
	BootFsckEnabled bool  `json:"boot_fsck_enabled"`
	InstallDir     string `json:"install_dir"`
	MountDir       string `json:"mount_dir"`
	TargetUser     string `json:"target_user"`
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
			Passphrase:      cfg.OfflineAP.Passphrase,
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
			BotToken:     cfg.Telegram.BotToken,
			ChatID:       cfg.Telegram.ChatID,
			OfflineMode:  cfg.Telegram.OfflineMode,
			MaxQueueSize: cfg.Telegram.MaxQueueSize,
			VideoQuality: cfg.Telegram.VideoQuality,
		},
		Update: updateConfigPublic{
			AutoUpdate:     cfg.Update.AutoUpdate,
			CheckOnStartup: cfg.Update.CheckOnStartup,
			Channel:        cfg.Update.Channel,
		},
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
	Network  *networkPatch  `json:"network,omitempty"`
	OfflineAP *offlineAPPatch `json:"offline_ap,omitempty"`
	Web      *webPatch      `json:"web,omitempty"`
	Telegram *telegramPatch `json:"telegram,omitempty"`
	Update   *updatePatch   `json:"update,omitempty"`
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

// Patch applies a partial update to the mutable config sections and persists to disk.
func (h *ConfigHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var req patchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cfg := h.cfg

	if p := req.Network; p != nil {
		if p.WebPort != nil {
			cfg.Network.WebPort = *p.WebPort
		}
	}

	if p := req.OfflineAP; p != nil {
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
			cfg.Update.CheckOnStartup = *p.CheckOnStartup
		}
		if p.Channel != nil {
			cfg.Update.Channel = *p.Channel
		}
	}

	if err := cfg.Save(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save config: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
