package boot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/services/chime"
	"github.com/ulm0/argus/internal/services/cleanup"
	"github.com/ulm0/argus/internal/services/mode"
)

// RunStartupSequence runs optional TeslaUSB-style boot steps before the web server is fully relied upon.
// It is non-blocking for the HTTP listener when BootBlock is false (caller runs this in a goroutine if desired).
func RunStartupSequence(ctx context.Context, cfg *config.Config, modeSvc *mode.Service, chSvc *chime.Service) {
	if !cfg.Installation.BootPresentOnStart {
		return
	}

	logger.L.Info("boot: startup sequence (present USB)")

	if cfg.DiskImages.BootFsckEnabled {
		runBootFsckQuick(cfg)
	}

	if cfg.Installation.BootCleanupOnStart {
		tryBootCleanup(cfg)
	}

	if cfg.Installation.BootRandomChimeOnStart {
		gm := chSvc.Groups()
		if gm.GetRandomConfig().Enabled {
			name := gm.SelectRandomChime("")
			if name != "" {
				if err := chSvc.ApplyScheduledChime(ctx, name); err != nil {
					logger.L.WithError(err).Warn("boot: random chime failed")
				}
			}
		}
	}

	if err := modeSvc.SwitchToPresent(); err != nil {
		logger.L.WithError(err).Error("boot: SwitchToPresent failed")
	}
}

// runBootFsckQuick runs a non-destructive fsck (-n) on disk image files before presenting (TeslaUSB boot parity).
func runBootFsckQuick(cfg *config.Config) {
	type partImg struct {
		n    int
		path string
	}
	var parts []partImg
	parts = append(parts, partImg{1, cfg.ImgCamPath})
	if cfg.DiskImages.Part2Enabled {
		parts = append(parts, partImg{2, cfg.ImgLightshow})
	}
	if cfg.DiskImages.MusicEnabled {
		parts = append(parts, partImg{3, cfg.ImgMusicPath})
	}
	for _, p := range parts {
		if p.path == "" {
			continue
		}
		if _, err := os.Stat(p.path); err != nil {
			continue
		}
		cmdName := "fsck.vfat"
		if p.n == 1 {
			cmdName = "fsck.exfat"
		}
		out, err := exec.Command(cmdName, "-n", p.path).CombinedOutput()
		if err != nil {
			logger.L.WithField("partition", p.n).WithError(err).WithField("output", string(out)).Warn("boot fsck reported issues")
			continue
		}
		logger.L.WithField("partition", p.n).Debug("boot fsck ok")
	}
}

func tryBootCleanup(cfg *config.Config) {
	cl := cleanup.NewService(cfg)
	policies := cl.GetPolicies()
	enabled := false
	for _, p := range policies {
		if p.Enabled && p.BootCleanup {
			enabled = true
			break
		}
	}
	if !enabled {
		return
	}

	part1 := filepath.Join(cfg.Installation.MountDir, "part1")
	if _, err := os.Stat(filepath.Join(part1, "TeslaCam")); err != nil {
		logger.L.Debug("boot cleanup skipped: TeslaCam not reachable (mount part1 first or run after edit mode)")
		return
	}

	plan, err := cl.CalculateCleanupPlan(part1)
	if err != nil {
		logger.L.WithError(err).Warn("boot cleanup: plan failed")
		return
	}
	rep := cl.ExecuteCleanup(plan, false)
	logger.L.WithField("deleted", rep.DeletedCount).Info("boot cleanup completed")
}
