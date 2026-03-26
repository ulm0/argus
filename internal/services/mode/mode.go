package mode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/system/gadget"
	"github.com/ulm0/argus/internal/system/loop"
	"github.com/ulm0/argus/internal/system/mount"
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
// unmounts all edit-mode partitions, assigns disk images to LUNs, and binds the gadget to UDC.
func (s *Service) SwitchToPresent() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mntMgr := mount.NewManager()
	loopMgr := loop.NewManager()
	gadgetMgr := gadget.NewManager()

	// Unmount all RW partitions and detach loop devices
	for _, part := range s.cfg.USBPartitions() {
		rwPath := s.cfg.MountPath(part, false)
		if mntMgr.IsMounted(rwPath) {
			if err := mntMgr.SafeUnmountDir(rwPath); err != nil {
				return fmt.Errorf("unmount %s: %w", rwPath, err)
			}
		}
	}
	mntMgr.Sync()

	// Detach loop devices for each image
	for _, imgPath := range s.enabledImagePaths() {
		_ = loopMgr.DetachAllForFile(imgPath)
	}

	// Build LUN configs for each enabled partition
	luns := s.buildLUNConfigs()

	// Create gadget structure if not present, then update LUN files
	if !gadgetMgr.IsPresent() {
		if err := gadgetMgr.Create(luns); err != nil {
			return fmt.Errorf("gadget create: %w", err)
		}
		if err := gadgetMgr.Bind(); err != nil {
			return fmt.Errorf("gadget bind: %w", err)
		}
	} else {
		// Already bound — just update LUN backing files
		for _, lun := range luns {
			if err := gadgetMgr.SetLUNFile(lun.Number, lun.File); err != nil {
				return fmt.Errorf("set lun %d: %w", lun.Number, err)
			}
		}
	}

	return s.writeStateFile("present")
}

// SwitchToEdit puts the system into "edit" mode:
// unbinds the gadget, clears LUN files, and mounts disk images as loop devices RW.
func (s *Service) SwitchToEdit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	gadgetMgr := gadget.NewManager()
	loopMgr := loop.NewManager()
	mntMgr := mount.NewManager()

	// Clear LUN backing files so the host releases the images
	if gadgetMgr.IsPresent() {
		for i := range s.buildLUNConfigs() {
			_ = gadgetMgr.ClearLUN(i)
		}
		time.Sleep(200 * time.Millisecond)
		if err := gadgetMgr.Unbind(); err != nil {
			return fmt.Errorf("gadget unbind: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Mount each enabled disk image RW via loop
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

	return s.writeStateFile("edit")
}

// buildLUNConfigs returns the LUN slice for the currently enabled partitions.
func (s *Service) buildLUNConfigs() []gadget.LUNConfig {
	var luns []gadget.LUNConfig
	idx := 0
	luns = append(luns, gadget.LUNConfig{Number: idx, File: s.cfg.ImgCamPath, ReadOnly: false, Removable: true})
	idx++
	if s.cfg.DiskImages.Part2Enabled {
		luns = append(luns, gadget.LUNConfig{Number: idx, File: s.cfg.ImgLightshow, ReadOnly: false, Removable: true})
		idx++
	}
	if s.cfg.DiskImages.MusicEnabled {
		luns = append(luns, gadget.LUNConfig{Number: idx, File: s.cfg.ImgMusicPath, ReadOnly: false, Removable: true})
	}
	return luns
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
