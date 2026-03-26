package video

import (
	"encoding/binary"
	"os"
	"path/filepath"
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

func TestParseSessionFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		session  string
		camera   string
		ok       bool
	}{
		{"2024-01-15_12-30-00-front.mp4", "2024-01-15_12-30-00", "front", true},
		{"2024-03-20_08-15-30-left_repeater.mp4", "2024-03-20_08-15-30", "left_repeater", true},
		{"2024-12-31_23-59-59-back.avi", "2024-12-31_23-59-59", "back", true},
		{"random_file.mp4", "", "", false},
		{"not-a-timestamp-camera.mp4", "", "", false},
		{"", "", "", false},
		{"2024-01-15_12-30-front.mp4", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			session, camera, ok := ParseSessionFromFilename(tt.filename)
			if ok != tt.ok {
				t.Errorf("ok = %v, want %v", ok, tt.ok)
			}
			if session != tt.session {
				t.Errorf("session = %q, want %q", session, tt.session)
			}
			if camera != tt.camera {
				t.Errorf("camera = %q, want %q", camera, tt.camera)
			}
		})
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{1099511627776, "1.00 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatFileSize(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatFileSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestIsValidMP4(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	dir := t.TempDir()

	t.Run("valid_mp4", func(t *testing.T) {
		// Minimal ftyp box: 4-byte size + "ftyp" + "isom"
		data := make([]byte, 20)
		binary.BigEndian.PutUint32(data[0:4], 20)
		copy(data[4:8], "ftyp")
		copy(data[8:12], "isom")

		path := filepath.Join(dir, "valid.mp4")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}

		if !svc.IsValidMP4(path) {
			t.Error("IsValidMP4() = false, want true for valid ftyp box")
		}
	})

	t.Run("invalid_mp4_wrong_magic", func(t *testing.T) {
		data := make([]byte, 20)
		binary.BigEndian.PutUint32(data[0:4], 20)
		copy(data[4:8], "moov")
		copy(data[8:12], "isom")

		path := filepath.Join(dir, "invalid.mp4")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}

		if svc.IsValidMP4(path) {
			t.Error("IsValidMP4() = true, want false for non-ftyp box")
		}
	})

	t.Run("invalid_mp4_too_small", func(t *testing.T) {
		path := filepath.Join(dir, "tiny.mp4")
		if err := os.WriteFile(path, []byte("hi"), 0644); err != nil {
			t.Fatal(err)
		}

		if svc.IsValidMP4(path) {
			t.Error("IsValidMP4() = true, want false for tiny file")
		}
	})

	t.Run("nonexistent_file", func(t *testing.T) {
		if svc.IsValidMP4(filepath.Join(dir, "nope.mp4")) {
			t.Error("IsValidMP4() = true, want false for missing file")
		}
	})
}

func TestGetFolders(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)

	// Create a TeslaCam directory structure on a simulated partition mount
	mntDir := cfg.MountDir
	part1RO := filepath.Join(mntDir, "part1-ro")
	tcPath := filepath.Join(part1RO, "TeslaCam")
	for _, sub := range []string{"SavedClips", "SentryClips", "RecentClips"} {
		if err := os.MkdirAll(filepath.Join(tcPath, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	folders := svc.GetFolders()
	if len(folders) != 3 {
		t.Fatalf("GetFolders() len = %d, want 3", len(folders))
	}

	names := make(map[string]bool)
	for _, f := range folders {
		names[f.Name] = true
	}
	for _, want := range []string{"SavedClips", "SentryClips", "RecentClips"} {
		if !names[want] {
			t.Errorf("GetFolders() missing folder %q", want)
		}
	}
}

func TestGetSessionVideos(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	dir := t.TempDir()

	files := []string{
		"2024-01-15_12-30-00-front.mp4",
		"2024-01-15_12-30-00-back.mp4",
		"2024-01-15_12-30-00-left_repeater.mp4",
		"2024-01-15_12-31-00-front.mp4",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	videos := svc.GetSessionVideos(dir, "2024-01-15_12-30-00")
	if len(videos) != 3 {
		t.Errorf("GetSessionVideos() len = %d, want 3", len(videos))
	}
}

func TestGroupVideosBySession(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	dir := t.TempDir()

	files := []string{
		"2024-01-15_12-30-00-front.mp4",
		"2024-01-15_12-30-00-back.mp4",
		"2024-01-16_08-00-00-front.mp4",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	groups, hasNext := svc.GroupVideosBySession(dir, 0, 10)
	if hasNext {
		t.Error("expected hasNext = false")
	}
	if len(groups) != 2 {
		t.Fatalf("GroupVideosBySession() len = %d, want 2", len(groups))
	}
	// Newest first
	if groups[0].Session != "2024-01-16_08-00-00" {
		t.Errorf("first session = %q, want %q", groups[0].Session, "2024-01-16_08-00-00")
	}
}

func TestGroupVideosBySessionPagination(t *testing.T) {
	cfg := testConfig(t)
	svc := NewService(cfg)
	dir := t.TempDir()

	for _, f := range []string{
		"2024-01-01_00-00-00-front.mp4",
		"2024-01-02_00-00-00-front.mp4",
		"2024-01-03_00-00-00-front.mp4",
	} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	groups, hasNext := svc.GroupVideosBySession(dir, 0, 2)
	if !hasNext {
		t.Error("expected hasNext = true for first page")
	}
	if len(groups) != 2 {
		t.Fatalf("page 0 len = %d, want 2", len(groups))
	}

	groups, hasNext = svc.GroupVideosBySession(dir, 1, 2)
	if hasNext {
		t.Error("expected hasNext = false for last page")
	}
	if len(groups) != 1 {
		t.Fatalf("page 1 len = %d, want 1", len(groups))
	}
}
