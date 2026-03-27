package mode

import (
	"testing"

	"github.com/ulm0/argus/internal/config"
)

func TestBuildLUNConfigs_Parity(t *testing.T) {
	cfg := &config.Config{
		ImgCamPath:   "/data/cam.img",
		ImgLightshow: "/data/lightshow.img",
		ImgMusicPath: "/data/music.img",
		DiskImages: config.DiskImagesConfig{
			Part2Enabled: true,
			MusicEnabled: true,
		},
	}
	s := NewService(cfg)
	luns := s.buildLUNConfigs()
	if len(luns) != 3 {
		t.Fatalf("len=%d want 3", len(luns))
	}
	if luns[0].ReadOnly {
		t.Error("LUN0 (TeslaCam) must be RW")
	}
	if luns[0].File != cfg.ImgCamPath || !luns[0].Removable {
		t.Errorf("LUN0: %v", luns[0])
	}
	if !luns[1].ReadOnly || luns[1].File != cfg.ImgLightshow {
		t.Errorf("LUN1 want RO lightshow: %v", luns[1])
	}
	if !luns[2].ReadOnly || luns[2].File != cfg.ImgMusicPath {
		t.Errorf("LUN2 want RO music: %v", luns[2])
	}
}

func TestBuildLUNConfigs_CamOnly(t *testing.T) {
	cfg := &config.Config{
		ImgCamPath: "/data/cam.img",
		DiskImages: config.DiskImagesConfig{
			Part2Enabled: false,
			MusicEnabled: false,
		},
	}
	s := NewService(cfg)
	luns := s.buildLUNConfigs()
	if len(luns) != 1 {
		t.Fatalf("len=%d want 1", len(luns))
	}
	if luns[0].ReadOnly {
		t.Error("LUN0 must be RW")
	}
}
