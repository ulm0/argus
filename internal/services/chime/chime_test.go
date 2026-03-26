package chime

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulm0/argus/internal/config"
)

func testConfig(t *testing.T) (*config.Config, string) {
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
  min_lock_chime_duration: 0.1
`
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, dir
}

// buildWAV creates a minimal 44-byte WAV header with optional data payload.
func buildWAV(sampleRate uint32, channels, bitsPerSample uint16, dataSize uint32) []byte {
	buf := make([]byte, 44+dataSize)

	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], 36+dataSize)
	copy(buf[8:12], "WAVE")

	// fmt sub-chunk
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16) // sub-chunk size
	binary.LittleEndian.PutUint16(buf[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(buf[22:24], channels)
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample/8)
	binary.LittleEndian.PutUint32(buf[28:32], byteRate)
	blockAlign := channels * bitsPerSample / 8
	binary.LittleEndian.PutUint16(buf[32:34], blockAlign)
	binary.LittleEndian.PutUint16(buf[34:36], bitsPerSample)

	// data sub-chunk
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], dataSize)

	return buf
}

func TestValidateTeslaWAV_Valid(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)
	dir := t.TempDir()

	tests := []struct {
		name       string
		sampleRate uint32
		channels   uint16
		dataSize   uint32
	}{
		{"44100_mono", 44100, 1, 44100 * 2},      // ~1s mono
		{"48000_mono", 48000, 1, 48000 * 2},       // ~1s mono
		{"44100_stereo", 44100, 2, 44100 * 2 * 2}, // ~1s stereo
		{"48000_stereo", 48000, 2, 48000 * 2 * 2}, // ~1s stereo
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := buildWAV(tt.sampleRate, tt.channels, 16, tt.dataSize)
			path := filepath.Join(dir, tt.name+".wav")
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}

			ok, msg := svc.ValidateTeslaWAV(path)
			if !ok {
				t.Errorf("ValidateTeslaWAV() = false: %s", msg)
			}
		})
	}
}

func TestValidateTeslaWAV_Invalid(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)
	dir := t.TempDir()

	t.Run("not_wav", func(t *testing.T) {
		path := filepath.Join(dir, "not_wav.wav")
		if err := os.WriteFile(path, []byte("this is not a wav file at all!!! needs 44 bytes"), 0644); err != nil {
			t.Fatal(err)
		}
		ok, msg := svc.ValidateTeslaWAV(path)
		if ok {
			t.Error("expected invalid for non-WAV file")
		}
		if msg == "" {
			t.Error("expected non-empty error message")
		}
	})

	t.Run("wrong_sample_rate", func(t *testing.T) {
		data := buildWAV(22050, 1, 16, 22050*2)
		path := filepath.Join(dir, "wrong_rate.wav")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		ok, _ := svc.ValidateTeslaWAV(path)
		if ok {
			t.Error("expected invalid for 22050 Hz sample rate")
		}
	})

	t.Run("wrong_bit_depth", func(t *testing.T) {
		data := buildWAV(44100, 1, 8, 44100)
		path := filepath.Join(dir, "wrong_bits.wav")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		ok, _ := svc.ValidateTeslaWAV(path)
		if ok {
			t.Error("expected invalid for 8-bit depth")
		}
	})

	t.Run("too_many_channels", func(t *testing.T) {
		data := buildWAV(44100, 6, 16, 44100*6*2)
		path := filepath.Join(dir, "too_many_ch.wav")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		ok, _ := svc.ValidateTeslaWAV(path)
		if ok {
			t.Error("expected invalid for 6 channels")
		}
	})

	t.Run("too_long", func(t *testing.T) {
		// 11 seconds at 44100 Hz, mono, 16-bit = 44100 * 2 * 11
		data := buildWAV(44100, 1, 16, 44100*2*11)
		path := filepath.Join(dir, "too_long.wav")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		ok, _ := svc.ValidateTeslaWAV(path)
		if ok {
			t.Error("expected invalid for >10s duration")
		}
	})

	t.Run("too_short", func(t *testing.T) {
		// Extremely short: 10 samples -> ~0.0002s at 44100 Hz
		data := buildWAV(44100, 1, 16, 20)
		path := filepath.Join(dir, "too_short.wav")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		ok, _ := svc.ValidateTeslaWAV(path)
		if ok {
			t.Error("expected invalid for very short duration")
		}
	})

	t.Run("too_small_header", func(t *testing.T) {
		path := filepath.Join(dir, "tiny.wav")
		if err := os.WriteFile(path, []byte("RIFF"), 0644); err != nil {
			t.Fatal(err)
		}
		ok, _ := svc.ValidateTeslaWAV(path)
		if ok {
			t.Error("expected invalid for truncated file")
		}
	})

	t.Run("nonexistent", func(t *testing.T) {
		ok, _ := svc.ValidateTeslaWAV(filepath.Join(dir, "nope.wav"))
		if ok {
			t.Error("expected invalid for missing file")
		}
	})

	t.Run("non_pcm_format", func(t *testing.T) {
		data := buildWAV(44100, 1, 16, 44100*2)
		// Overwrite audioFormat to 3 (IEEE float)
		binary.LittleEndian.PutUint16(data[20:22], 3)
		path := filepath.Join(dir, "non_pcm.wav")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		ok, _ := svc.ValidateTeslaWAV(path)
		if ok {
			t.Error("expected invalid for non-PCM format")
		}
	})
}

func TestSchedulerCRUD(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)
	sched := svc.Scheduler()

	// List should be empty initially
	all := sched.ListSchedules(false)
	if len(all) != 0 {
		t.Fatalf("initial schedules len = %d, want 0", len(all))
	}

	// Add
	id, err := sched.AddSchedule(Schedule{
		ChimeFilename: "test.wav",
		Time:          "08:00",
		Type:          ScheduleWeekly,
		Enabled:       true,
		Name:          "morning",
	})
	if err != nil {
		t.Fatalf("AddSchedule() error: %v", err)
	}
	if id == "" {
		t.Fatal("AddSchedule() returned empty ID")
	}

	// Get
	got := sched.GetSchedule(id)
	if got == nil {
		t.Fatal("GetSchedule() returned nil for existing schedule")
	}
	if got.Name != "morning" {
		t.Errorf("Name = %q, want %q", got.Name, "morning")
	}
	if got.ChimeFilename != "test.wav" {
		t.Errorf("ChimeFilename = %q, want %q", got.ChimeFilename, "test.wav")
	}

	// List
	all = sched.ListSchedules(false)
	if len(all) != 1 {
		t.Fatalf("after add, schedules len = %d, want 1", len(all))
	}

	// List enabled only
	enabled := sched.ListSchedules(true)
	if len(enabled) != 1 {
		t.Fatalf("enabled schedules len = %d, want 1", len(enabled))
	}

	// Update
	if err := sched.UpdateSchedule(id, map[string]any{"name": "evening", "enabled": false}); err != nil {
		t.Fatalf("UpdateSchedule() error: %v", err)
	}
	got = sched.GetSchedule(id)
	if got.Name != "evening" {
		t.Errorf("after update, Name = %q, want %q", got.Name, "evening")
	}
	if got.Enabled {
		t.Error("after update, Enabled = true, want false")
	}

	// List enabled only should be empty
	enabled = sched.ListSchedules(true)
	if len(enabled) != 0 {
		t.Fatalf("enabled schedules after disable = %d, want 0", len(enabled))
	}

	// Update non-existent
	if err := sched.UpdateSchedule("nonexistent", map[string]any{}); err == nil {
		t.Error("UpdateSchedule() expected error for nonexistent ID")
	}

	// Delete
	if err := sched.DeleteSchedule(id); err != nil {
		t.Fatalf("DeleteSchedule() error: %v", err)
	}
	all = sched.ListSchedules(false)
	if len(all) != 0 {
		t.Fatalf("after delete, schedules len = %d, want 0", len(all))
	}

	// Delete non-existent
	if err := sched.DeleteSchedule("nonexistent"); err == nil {
		t.Error("DeleteSchedule() expected error for nonexistent ID")
	}
}

func TestSchedulerGetNonExistent(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)
	sched := svc.Scheduler()

	if sched.GetSchedule("nonexistent") != nil {
		t.Error("GetSchedule() expected nil for nonexistent ID")
	}
}

func TestGroupManagerCRUD(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)
	gm := svc.Groups()

	// Initially empty
	groups := gm.ListGroups()
	if len(groups) != 0 {
		t.Fatalf("initial groups len = %d, want 0", len(groups))
	}

	// Create
	id, err := gm.CreateGroup("Holidays", "Holiday chimes", []string{"xmas.wav", "newyear.wav"})
	if err != nil {
		t.Fatalf("CreateGroup() error: %v", err)
	}
	if id == "" {
		t.Fatal("CreateGroup() returned empty ID")
	}

	groups = gm.ListGroups()
	if len(groups) != 1 {
		t.Fatalf("after create, groups len = %d, want 1", len(groups))
	}
	if groups[0].Name != "Holidays" {
		t.Errorf("Name = %q, want %q", groups[0].Name, "Holidays")
	}
	if len(groups[0].Chimes) != 2 {
		t.Errorf("Chimes len = %d, want 2", len(groups[0].Chimes))
	}

	// Update
	if err := gm.UpdateGroup(id, "Updated", "new desc", []string{"single.wav"}); err != nil {
		t.Fatalf("UpdateGroup() error: %v", err)
	}
	groups = gm.ListGroups()
	if groups[0].Name != "Updated" {
		t.Errorf("after update, Name = %q, want %q", groups[0].Name, "Updated")
	}
	if len(groups[0].Chimes) != 1 {
		t.Errorf("after update, Chimes len = %d, want 1", len(groups[0].Chimes))
	}

	// Update non-existent
	if err := gm.UpdateGroup("nonexistent", "x", "x", nil); err == nil {
		t.Error("UpdateGroup() expected error for nonexistent ID")
	}

	// Add chime to group
	if err := gm.AddChimeToGroup(id, "extra.wav"); err != nil {
		t.Fatalf("AddChimeToGroup() error: %v", err)
	}
	groups = gm.ListGroups()
	if len(groups[0].Chimes) != 2 {
		t.Errorf("after add chime, Chimes len = %d, want 2", len(groups[0].Chimes))
	}

	// Add duplicate (should be idempotent)
	if err := gm.AddChimeToGroup(id, "extra.wav"); err != nil {
		t.Fatalf("AddChimeToGroup() duplicate error: %v", err)
	}
	groups = gm.ListGroups()
	if len(groups[0].Chimes) != 2 {
		t.Errorf("after duplicate add, Chimes len = %d, want 2", len(groups[0].Chimes))
	}

	// Remove chime from group
	if err := gm.RemoveChimeFromGroup(id, "extra.wav"); err != nil {
		t.Fatalf("RemoveChimeFromGroup() error: %v", err)
	}
	groups = gm.ListGroups()
	if len(groups[0].Chimes) != 1 {
		t.Errorf("after remove chime, Chimes len = %d, want 1", len(groups[0].Chimes))
	}

	// Delete
	if err := gm.DeleteGroup(id); err != nil {
		t.Fatalf("DeleteGroup() error: %v", err)
	}
	groups = gm.ListGroups()
	if len(groups) != 0 {
		t.Fatalf("after delete, groups len = %d, want 0", len(groups))
	}

	// Delete non-existent
	if err := gm.DeleteGroup("nonexistent"); err == nil {
		t.Error("DeleteGroup() expected error for nonexistent ID")
	}
}

func TestSelectRandomChime(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)
	gm := svc.Groups()

	// Random disabled
	result := gm.SelectRandomChime("")
	if result != "" {
		t.Errorf("SelectRandomChime() = %q, want empty when disabled", result)
	}

	// Create group
	id, err := gm.CreateGroup("Pool", "Random pool", []string{"a.wav", "b.wav", "c.wav"})
	if err != nil {
		t.Fatal(err)
	}

	// Enable random mode
	if err := gm.SetRandomMode(true, id); err != nil {
		t.Fatal(err)
	}

	rc := gm.GetRandomConfig()
	if !rc.Enabled || rc.GroupID != id {
		t.Errorf("RandomConfig = %+v, want enabled with groupID %q", rc, id)
	}

	// Select a random chime
	selected := gm.SelectRandomChime("")
	if selected == "" {
		t.Fatal("SelectRandomChime() returned empty")
	}
	valid := map[string]bool{"a.wav": true, "b.wav": true, "c.wav": true}
	if !valid[selected] {
		t.Errorf("SelectRandomChime() = %q, not in expected set", selected)
	}

	// Select avoiding a specific chime (with multiple runs to verify it's from the set)
	for i := 0; i < 20; i++ {
		s := gm.SelectRandomChime("a.wav")
		if s == "" {
			t.Fatal("SelectRandomChime(avoid) returned empty")
		}
		// Should still be from the valid set
		if !valid[s] {
			t.Errorf("SelectRandomChime(avoid) = %q, not in valid set", s)
		}
	}

	// Random with empty group
	id2, _ := gm.CreateGroup("Empty", "Empty group", []string{})
	if err := gm.SetRandomMode(true, id2); err != nil {
		t.Fatal(err)
	}
	result = gm.SelectRandomChime("")
	if result != "" {
		t.Errorf("SelectRandomChime() from empty group = %q, want empty", result)
	}
}

func TestListChimes(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)
	dir := t.TempDir()

	chimesDir := filepath.Join(dir, cfg.Web.ChimesFolder)
	if err := os.MkdirAll(chimesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create some chime files
	for _, name := range []string{"chime1.wav", "chime2.wav", "not_a_chime.txt"} {
		if err := os.WriteFile(filepath.Join(chimesDir, name), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	files := svc.ListChimes(dir)
	if len(files) != 2 {
		t.Errorf("ListChimes() len = %d, want 2 (only .wav)", len(files))
	}
}

func TestGetActiveChimeInfo(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)
	dir := t.TempDir()

	// No active chime
	name, exists := svc.GetActiveChimeInfo(dir)
	if exists {
		t.Error("expected no active chime")
	}

	// Create active chime
	if err := os.WriteFile(filepath.Join(dir, cfg.Web.LockChimeFilename), []byte("wav"), 0644); err != nil {
		t.Fatal(err)
	}
	name, exists = svc.GetActiveChimeInfo(dir)
	if !exists {
		t.Error("expected active chime to exist")
	}
	if name != cfg.Web.LockChimeFilename {
		t.Errorf("active chime name = %q, want %q", name, cfg.Web.LockChimeFilename)
	}
}
