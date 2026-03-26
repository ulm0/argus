package wrap

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulm0/argus/internal/config"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "gadget", "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}
	yaml := `
installation:
  target_user: pi
  mount_dir: ` + filepath.Join(dir, "mnt") + `
disk_images:
  cam_name: usb_cam.img
  lightshow_name: usb_lightshow.img
network:
  samba_password: pw
web:
  max_lock_chime_size: 1048576
  max_lock_chime_duration: 10.0
  min_lock_chime_duration: 0.5
`
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// buildPNG creates a minimal PNG byte slice with the specified dimensions.
// It includes the 8-byte PNG signature + 4-byte IHDR length + 4-byte "IHDR" +
// 4-byte width + 4-byte height = 24 bytes minimum.
func buildPNG(width, height int) []byte {
	buf := make([]byte, 24)
	// PNG signature
	copy(buf[0:8], []byte{137, 80, 78, 71, 13, 10, 26, 10})
	// IHDR chunk length (13 bytes for a full IHDR, but we only need w/h for parsing)
	binary.BigEndian.PutUint32(buf[8:12], 13)
	copy(buf[12:16], "IHDR")
	binary.BigEndian.PutUint32(buf[16:20], uint32(width))
	binary.BigEndian.PutUint32(buf[20:24], uint32(height))
	return buf
}

func TestGetPNGDimensionsFromBytes_Valid(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
	}{
		{"512x512", 512, 512},
		{"1024x1024", 1024, 1024},
		{"800x600", 800, 600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildPNG(tt.width, tt.height)
			w, h, err := GetPNGDimensionsFromBytes(data)
			if err != nil {
				t.Fatalf("GetPNGDimensionsFromBytes() error: %v", err)
			}
			if w != tt.width || h != tt.height {
				t.Errorf("got %dx%d, want %dx%d", w, h, tt.width, tt.height)
			}
		})
	}
}

func TestGetPNGDimensionsFromBytes_Invalid(t *testing.T) {
	t.Run("too_short", func(t *testing.T) {
		_, _, err := GetPNGDimensionsFromBytes([]byte{137, 80, 78, 71})
		if err == nil {
			t.Error("expected error for data too short")
		}
	})

	t.Run("not_png", func(t *testing.T) {
		data := make([]byte, 24)
		copy(data[0:4], "JFIF")
		_, _, err := GetPNGDimensionsFromBytes(data)
		if err == nil {
			t.Error("expected error for non-PNG signature")
		}
	})

	t.Run("empty", func(t *testing.T) {
		_, _, err := GetPNGDimensionsFromBytes(nil)
		if err == nil {
			t.Error("expected error for nil data")
		}
	})
}

func TestGetPNGDimensions_File(t *testing.T) {
	dir := t.TempDir()

	data := buildPNG(768, 768)
	path := filepath.Join(dir, "test.png")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	w, h, err := GetPNGDimensions(path)
	if err != nil {
		t.Fatalf("GetPNGDimensions() error: %v", err)
	}
	if w != 768 || h != 768 {
		t.Errorf("got %dx%d, want 768x768", w, h)
	}

	// Non-existent file
	_, _, err = GetPNGDimensions(filepath.Join(dir, "nonexistent.png"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestUploadWrap_Valid(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	data := buildPNG(512, 512)
	err := svc.UploadWrap(data, "test_wrap.png", mountPath)
	if err != nil {
		t.Fatalf("UploadWrap() error: %v", err)
	}

	saved := filepath.Join(mountPath, WrapsFolder, "test_wrap.png")
	if _, err := os.Stat(saved); err != nil {
		t.Errorf("uploaded file not found: %v", err)
	}
}

func TestUploadWrap_FilenameTooLong(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	longName := strings.Repeat("a", MaxFilenameLen+1) + ".png"
	data := buildPNG(512, 512)

	err := svc.UploadWrap(data, longName, mountPath)
	if err == nil {
		t.Error("expected error for filename too long")
	}
}

func TestUploadWrap_NotPNG(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	data := buildPNG(512, 512)
	err := svc.UploadWrap(data, "test.jpg", mountPath)
	if err == nil {
		t.Error("expected error for non-PNG extension")
	}
}

func TestUploadWrap_TooLarge(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	data := make([]byte, MaxFileSize+1)
	copy(data, buildPNG(512, 512))

	err := svc.UploadWrap(data, "big.png", mountPath)
	if err == nil {
		t.Error("expected error for file too large")
	}
}

func TestUploadWrap_DimensionsTooSmall(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	data := buildPNG(100, 100)
	err := svc.UploadWrap(data, "small.png", mountPath)
	if err == nil {
		t.Error("expected error for dimensions too small")
	}
}

func TestUploadWrap_DimensionsTooLarge(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	data := buildPNG(2048, 2048)
	err := svc.UploadWrap(data, "huge.png", mountPath)
	if err == nil {
		t.Error("expected error for dimensions too large")
	}
}

func TestUploadWrap_CountLimit(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	wrapsDir := filepath.Join(mountPath, WrapsFolder)
	if err := os.MkdirAll(wrapsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Pre-fill with MaxWrapCount PNG files
	for i := 0; i < MaxWrapCount; i++ {
		name := filepath.Join(wrapsDir, strings.Repeat("x", 5)+string(rune('a'+i))+".png")
		if err := os.WriteFile(name, buildPNG(512, 512), 0644); err != nil {
			t.Fatal(err)
		}
	}

	data := buildPNG(512, 512)
	err := svc.UploadWrap(data, "overflow.png", mountPath)
	if err == nil {
		t.Error("expected error when wrap count limit exceeded")
	}
}

func TestGetWrapCount(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	// Empty directory
	if count := svc.GetWrapCount(mountPath); count != 0 {
		t.Errorf("empty GetWrapCount() = %d, want 0", count)
	}

	wrapsDir := filepath.Join(mountPath, WrapsFolder)
	if err := os.MkdirAll(wrapsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Add PNG and non-PNG files
	os.WriteFile(filepath.Join(wrapsDir, "a.png"), []byte("png"), 0644)
	os.WriteFile(filepath.Join(wrapsDir, "b.png"), []byte("png"), 0644)
	os.WriteFile(filepath.Join(wrapsDir, "c.txt"), []byte("txt"), 0644)

	if count := svc.GetWrapCount(mountPath); count != 2 {
		t.Errorf("GetWrapCount() = %d, want 2", count)
	}
}

func TestListWraps(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	wrapsDir := filepath.Join(mountPath, WrapsFolder)
	if err := os.MkdirAll(wrapsDir, 0755); err != nil {
		t.Fatal(err)
	}

	pngData := buildPNG(800, 600)
	if err := os.WriteFile(filepath.Join(wrapsDir, "wrap1.png"), pngData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wrapsDir, "not_png.txt"), []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}

	files := svc.ListWraps(mountPath)
	if len(files) != 1 {
		t.Fatalf("ListWraps() len = %d, want 1", len(files))
	}
	if files[0].Filename != "wrap1.png" {
		t.Errorf("Filename = %q, want %q", files[0].Filename, "wrap1.png")
	}
	if files[0].Width != 800 || files[0].Height != 600 {
		t.Errorf("dimensions = %dx%d, want 800x600", files[0].Width, files[0].Height)
	}
}

func TestDeleteWrap(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	mountPath := t.TempDir()

	wrapsDir := filepath.Join(mountPath, WrapsFolder)
	if err := os.MkdirAll(wrapsDir, 0755); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(wrapsDir, "to_delete.png")
	if err := os.WriteFile(filePath, buildPNG(512, 512), 0644); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteWrap("to_delete.png", mountPath); err != nil {
		t.Fatalf("DeleteWrap() error: %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}

	// Delete non-existent
	if err := svc.DeleteWrap("nonexistent.png", mountPath); err == nil {
		t.Error("expected error for deleting nonexistent file")
	}
}
