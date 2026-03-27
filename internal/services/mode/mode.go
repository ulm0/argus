package mode

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/system/gadget"
	"github.com/ulm0/argus/internal/system/loop"
	"github.com/ulm0/argus/internal/system/mount"
	"github.com/ulm0/argus/internal/system/samba"
)

type ModeInfo struct {
	Token      string            `json:"mode"`
	Label      string            `json:"mode_label"`
	CSSClass   string            `json:"mode_class"`
	SharePaths map[string]string `json:"share_paths,omitempty"`
}

type Service struct {
	cfg *config.Config
	mu  sync.RWMutex
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) CurrentMode() ModeInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	token := s.readStateFile()
	if token == "" {
		token = s.detectMode()
	}

	switch token {
	case "present":
		return ModeInfo{Token: "present", Label: "USB Gadget Mode", CSSClass: "present"}
	case "edit":
		shares := make(map[string]string)
		hostname, _ := os.Hostname()
		for _, part := range s.cfg.USBPartitions() {
			shares[part] = fmt.Sprintf("\\\\%s\\%s", hostname, part)
		}
		return ModeInfo{Token: "edit", Label: "Edit Mode", CSSClass: "edit", SharePaths: shares}
	default:
		return ModeInfo{Token: "unknown", Label: "Unknown", CSSClass: "unknown"}
	}
}

// SwitchToPresent puts the USB gadget into "present" mode:
// unmounts all edit-mode partitions, assigns disk images to LUNs, binds the gadget to UDC,
// and mounts local read-only views (TeslaUSB parity).
func (s *Service) SwitchToPresent() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.prepareSystemForPresent(); err != nil {
		return fmt.Errorf("prepare present: %w", err)
	}

	mntMgr := mount.NewManager()
	loopMgr := loop.NewManager()
	gadgetMgr := gadget.NewManager()

	for _, part := range s.cfg.USBPartitions() {
		roPath := s.cfg.MountPath(part, true)
		if mntMgr.IsMounted(roPath) {
			if err := mntMgr.SafeUnmountDir(roPath); err != nil {
				return fmt.Errorf("unmount %s: %w", roPath, err)
			}
		}
	}
	for _, part := range s.cfg.USBPartitions() {
		rwPath := s.cfg.MountPath(part, false)
		if mntMgr.IsMounted(rwPath) {
			if err := mntMgr.SafeUnmountDir(rwPath); err != nil {
				return fmt.Errorf("unmount %s: %w", rwPath, err)
			}
		}
	}
	mntMgr.Sync()

	for _, imgPath := range s.enabledImagePaths() {
		_ = loopMgr.DetachAllForFile(imgPath)
	}

	luns := s.buildLUNConfigs()
	serial, err := gadget.LoadOrCreateSerial(s.cfg.GadgetDir)
	if err != nil {
		return fmt.Errorf("usb serial: %w", err)
	}

	if _, err := os.Stat(gadgetMgr.GadgetDir()); err == nil {
		if err := gadgetMgr.Remove(); err != nil {
			logger.L.WithError(err).Warn("gadget remove before recreate")
		}
	}

	if err := gadgetMgr.Create(luns, serial); err != nil {
		return fmt.Errorf("gadget create: %w", err)
	}
	if err := gadgetMgr.Bind(); err != nil {
		return fmt.Errorf("gadget bind: %w", err)
	}

	if err := s.mountPresentReadOnlyLocal(mntMgr, loopMgr); err != nil {
		return err
	}

	return s.writeStateFile("present")
}

// SwitchToEdit puts the system into "edit" mode:
// unbinds the gadget, clears LUN files, unmounts present-mode RO mounts, and mounts RW for Samba.
func (s *Service) SwitchToEdit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.waitQuickEditLocks(quickEditLockTimeout); err != nil {
		return fmt.Errorf("prepare edit: %w", err)
	}

	gadgetMgr := gadget.NewManager()
	loopMgr := loop.NewManager()
	mntMgr := mount.NewManager()

	if gadgetMgr.IsPresent() {
		for _, lun := range s.buildLUNConfigs() {
			_ = gadgetMgr.ClearLUN(lun.Number)
		}
		time.Sleep(200 * time.Millisecond)
		if err := gadgetMgr.Unbind(); err != nil {
			return fmt.Errorf("gadget unbind: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	for _, part := range s.cfg.USBPartitions() {
		roPath := s.cfg.MountPath(part, true)
		if mntMgr.IsMounted(roPath) {
			if err := mntMgr.SafeUnmountDir(roPath); err != nil {
				return fmt.Errorf("unmount RO %s: %w", roPath, err)
			}
		}
	}
	mntMgr.Sync()
	for _, imgPath := range s.enabledImagePaths() {
		_ = loopMgr.DetachAllForFile(imgPath)
	}

	pairs := s.enabledPartitionImagePairs()
	for _, pi := range pairs {
		mntPoint := s.cfg.MountPath(pi.part, false)
		if err := os.MkdirAll(mntPoint, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", mntPoint, err)
		}
		if mntMgr.IsMounted(mntPoint) {
			continue
		}

		loopDev, err := loopMgr.Create(pi.imgPath, false)
		if err != nil {
			return fmt.Errorf("loop %s: %w", pi.imgPath, err)
		}

		fsType, err := mntMgr.DetectFSType(loopDev)
		if err != nil || fsType == "" {
			fsType = "auto"
		}
		if err := mntMgr.Mount(loopDev, mntPoint, fsType, false); err != nil {
			_ = loopMgr.Detach(loopDev)
			return fmt.Errorf("mount %s -> %s: %w", loopDev, mntPoint, err)
		}
	}

	sm := samba.NewManager(s.cfg)
	if err := sm.RestartSambaServices(); err != nil {
		logger.L.WithError(err).Warn("restart Samba after edit mode")
	}

	return s.writeStateFile("edit")
}

// buildLUNConfigs returns the LUN slice for the currently enabled partitions.
// LUN0 TeslaCam is RW; Lightshow and Music LUNs are RO (TeslaUSB parity).
func (s *Service) buildLUNConfigs() []gadget.LUNConfig {
	var luns []gadget.LUNConfig
	idx := 0
	luns = append(luns, gadget.LUNConfig{Number: idx, File: s.cfg.ImgCamPath, ReadOnly: false, Removable: true})
	idx++
	if s.cfg.DiskImages.Part2Enabled {
		luns = append(luns, gadget.LUNConfig{Number: idx, File: s.cfg.ImgLightshow, ReadOnly: true, Removable: true})
		idx++
	}
	if s.cfg.DiskImages.MusicEnabled {
		luns = append(luns, gadget.LUNConfig{Number: idx, File: s.cfg.ImgMusicPath, ReadOnly: true, Removable: true})
	}
	return luns
}

func targetUserUIDGID(cfg *config.Config) (uid, gid int, err error) {
	u, err := user.Lookup(cfg.Installation.TargetUser)
	if err != nil {
		return 0, 0, err
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func (s *Service) mountPresentReadOnlyLocal(mntMgr *mount.Manager, loopMgr *loop.Manager) error {
	uid, gid, err := targetUserUIDGID(s.cfg)
	if err != nil {
		logger.L.WithError(err).Warn("target user UID/GID; using 0 for RO mounts")
	}

	for _, pi := range s.enabledPartitionImagePairs() {
		roPath := s.cfg.MountPath(pi.part, true)
		if mntMgr.IsMounted(roPath) {
			continue
		}
		loopDev, err := loopMgr.Create(pi.imgPath, true)
		if err != nil {
			return fmt.Errorf("loop RO %s: %w", pi.part, err)
		}
		fsType, err := mntMgr.DetectFSType(loopDev)
		if err != nil || fsType == "" {
			fsType = "vfat"
		}
		switch fsType {
		case "vfat", "exfat":
			if err := mntMgr.MountLoopReadOnlyUser(loopDev, roPath, fsType, uid, gid); err != nil {
				_ = loopMgr.Detach(loopDev)
				return fmt.Errorf("mount RO %s: %w", pi.part, err)
			}
		default:
			if err := mntMgr.Mount(loopDev, roPath, fsType, true); err != nil {
				_ = loopMgr.Detach(loopDev)
				return fmt.Errorf("mount RO %s: %w", pi.part, err)
			}
		}
	}
	return nil
}

// enabledImagePaths returns image file paths for all currently enabled partitions.
func (s *Service) enabledImagePaths() []string {
	paths := []string{s.cfg.ImgCamPath}
	if s.cfg.DiskImages.Part2Enabled {
		paths = append(paths, s.cfg.ImgLightshow)
	}
	if s.cfg.DiskImages.MusicEnabled {
		paths = append(paths, s.cfg.ImgMusicPath)
	}
	return paths
}

// enabledPartitionImagePairs returns (partition name, image path) for all enabled partitions.
func (s *Service) enabledPartitionImagePairs() []struct{ part, imgPath string } {
	pairs := []struct{ part, imgPath string }{
		{"part1", s.cfg.ImgCamPath},
	}
	if s.cfg.DiskImages.Part2Enabled {
		pairs = append(pairs, struct{ part, imgPath string }{"part2", s.cfg.ImgLightshow})
	}
	if s.cfg.DiskImages.MusicEnabled {
		pairs = append(pairs, struct{ part, imgPath string }{"part3", s.cfg.ImgMusicPath})
	}
	return pairs
}

// writeStateFile writes the current mode token to the state file.
func (s *Service) writeStateFile(token string) error {
	return os.WriteFile(s.cfg.StateFile, []byte(token), 0644)
}

func (s *Service) FeatureAvailability() map[string]bool {
	features := make(map[string]bool)
	part2exists := s.cfg.DiskImages.Part2Enabled && fileExists(s.cfg.ImgLightshow)
	features["videos_available"] = fileExists(s.cfg.ImgCamPath)
	features["analytics_available"] = fileExists(s.cfg.ImgCamPath)
	features["chimes_available"] = part2exists && s.cfg.DiskImages.ChimesEnabled
	features["shows_available"] = part2exists && s.cfg.DiskImages.LightshowEnabled
	features["wraps_available"] = part2exists && s.cfg.DiskImages.WrapsEnabled
	features["music_available"] = s.cfg.DiskImages.MusicEnabled && fileExists(s.cfg.ImgMusicPath)
	return features
}

func (s *Service) Hostname() string {
	h, _ := os.Hostname()
	return h
}

func (s *Service) GadgetState() map[string]any {
	state := map[string]any{
		"mode": s.CurrentMode().Token,
	}

	// Check configfs for gadget presence
	lunFile := "/sys/kernel/config/usb_gadget/argus/functions/mass_storage.usb0/lun.0/file"
	if data, err := os.ReadFile(lunFile); err == nil {
		state["lun0_file"] = strings.TrimSpace(string(data))
		state["gadget_present"] = true
	} else {
		state["gadget_present"] = false
	}
	return state
}

func (s *Service) RecoverGadget() (map[string]any, error) {
	// Placeholder for gadget recovery logic
	return map[string]any{"recovered": false, "message": "recovery not yet implemented"}, nil
}

func (s *Service) OperationStatus() map[string]any {
	lockFiles := []string{
		filepath.Join(s.cfg.GadgetDir, ".quick_edit_part2.lock"),
		filepath.Join(s.cfg.GadgetDir, ".quick_edit_part3.lock"),
	}

	for _, lock := range lockFiles {
		if info, err := os.Stat(lock); err == nil {
			age := time.Since(info.ModTime())
			return map[string]any{
				"in_progress":          true,
				"lock_age":             age.Seconds(),
				"estimated_completion": 120 - age.Seconds(),
			}
		}
	}
	return map[string]any{"in_progress": false}
}

func (s *Service) readStateFile() string {
	data, err := os.ReadFile(s.cfg.StateFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *Service) detectMode() string {
	// Check if configfs gadget is present
	matches, _ := filepath.Glob("/sys/kernel/config/usb_gadget/*/functions/mass_storage.usb0/lun.0/file")
	if len(matches) > 0 {
		return "present"
	}

	// Check if g_mass_storage is loaded
	data, err := os.ReadFile("/proc/modules")
	if err == nil && strings.Contains(string(data), "g_mass_storage") {
		return "present"
	}

	// Check if partitions are mounted RW (edit mode)
	for _, part := range s.cfg.USBPartitions() {
		mountPath := s.cfg.MountPath(part, false)
		if isMount(mountPath) {
			return "edit"
		}
	}

	return "unknown"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isMount(path string) bool {
	mntMgr := mount.NewManager()
	return mntMgr.IsMounted(path)
}
