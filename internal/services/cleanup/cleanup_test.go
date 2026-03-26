package cleanup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	return cfg, dir
}

// createMockVideoFiles creates mock .mp4 files of given sizes in the target directory,
// with modification times spaced by -24h each (oldest last).
func createMockVideoFiles(t *testing.T, dir string, count int, sizeBytes int) []string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var names []string
	now := time.Now()
	for i := 0; i < count; i++ {
		name := filepath.Join(dir, "video_"+time.Now().Format("20060102")+
			"_"+string(rune('a'+i))+".mp4")
		data := make([]byte, sizeBytes)
		if err := os.WriteFile(name, data, 0644); err != nil {
			t.Fatal(err)
		}
		// Set mod time: newest first (i=0 is newest)
		modTime := now.Add(-time.Duration(i) * 24 * time.Hour)
		if err := os.Chtimes(name, modTime, modTime); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return names
}

func TestSaveAndLoadPolicies(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	// Initially empty
	policies := svc.GetPolicies()
	if len(policies) != 0 {
		t.Fatalf("initial policies len = %d, want 0", len(policies))
	}

	// Save policies
	newPolicies := map[string]FolderPolicy{
		"SavedClips": {
			Enabled:     true,
			BootCleanup: true,
			AgeBased:    &AgePolicy{Enabled: true, MaxDays: 30},
		},
		"SentryClips": {
			Enabled:   true,
			SizeBased: &SizePolicy{Enabled: true, MaxGB: 5.0},
		},
	}
	if err := svc.SavePolicies(newPolicies); err != nil {
		t.Fatalf("SavePolicies() error: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(svc.configFile)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	var loaded map[string]FolderPolicy
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal saved policies: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("saved policies len = %d, want 2", len(loaded))
	}
	if !loaded["SavedClips"].AgeBased.Enabled {
		t.Error("SavedClips AgeBased should be enabled")
	}

	// Reload by creating a new service instance
	svc2 := NewService(cfg)
	policies2 := svc2.GetPolicies()
	if len(policies2) != 2 {
		t.Errorf("reloaded policies len = %d, want 2", len(policies2))
	}
	if policies2["SavedClips"].AgeBased == nil || policies2["SavedClips"].AgeBased.MaxDays != 30 {
		t.Error("reloaded SavedClips AgeBased policy mismatch")
	}
}

func TestCalculateCleanupPlan_AgePolicy(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	createMockVideoFiles(t, folderPath, 5, 1024)

	// Set age policy: max 2 days old
	svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {
			Enabled:  true,
			AgeBased: &AgePolicy{Enabled: true, MaxDays: 2},
		},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan() error: %v", err)
	}

	// Files at 0, -1, -2, -3, -4 days. Files at -3d and -4d should be marked.
	if plan.TotalCount < 2 {
		t.Errorf("TotalCount = %d, want >= 2 (files older than 2 days)", plan.TotalCount)
	}

	files, ok := plan.Breakdown["SavedClips"]
	if !ok {
		t.Fatal("Breakdown missing SavedClips")
	}
	for _, f := range files {
		if f.Reason == "" {
			t.Error("file to delete has empty reason")
		}
	}
}

func TestCalculateCleanupPlan_SizePolicy(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SentryClips")
	// 5 files of 1KB each = 5KB total
	createMockVideoFiles(t, folderPath, 5, 1024)

	// Max 3KB -> should delete oldest files to get under 3KB
	maxGB := 3.0 / (1024 * 1024 * 1024)
	svc.SavePolicies(map[string]FolderPolicy{
		"SentryClips": {
			Enabled:   true,
			SizeBased: &SizePolicy{Enabled: true, MaxGB: maxGB},
		},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan() error: %v", err)
	}

	// 5KB total, 3KB max -> need to delete at least 2 files (oldest first)
	if plan.TotalCount < 2 {
		t.Errorf("TotalCount = %d, want >= 2 for size policy", plan.TotalCount)
	}
}

func TestCalculateCleanupPlan_CountPolicy(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "RecentClips")
	createMockVideoFiles(t, folderPath, 5, 512)

	// Max 3 files
	svc.SavePolicies(map[string]FolderPolicy{
		"RecentClips": {
			Enabled:    true,
			CountBased: &CountPolicy{Enabled: true, MaxCount: 3},
		},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan() error: %v", err)
	}

	if plan.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2 (5 files - 3 max)", plan.TotalCount)
	}
}

func TestCalculateCleanupPlan_DisabledPolicy(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	createMockVideoFiles(t, folderPath, 5, 512)

	svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {
			Enabled:    false,
			CountBased: &CountPolicy{Enabled: true, MaxCount: 1},
		},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan() error: %v", err)
	}

	if plan.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0 for disabled policy", plan.TotalCount)
	}
}

func TestCalculateCleanupPlan_NonExistentFolder(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()

	svc.SavePolicies(map[string]FolderPolicy{
		"MissingFolder": {
			Enabled:    true,
			CountBased: &CountPolicy{Enabled: true, MaxCount: 1},
		},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan() error: %v", err)
	}

	if plan.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0 for non-existent folder", plan.TotalCount)
	}
}

func TestExecuteCleanup_DryRun(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	files := createMockVideoFiles(t, folderPath, 5, 512)

	svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {
			Enabled:    true,
			CountBased: &CountPolicy{Enabled: true, MaxCount: 3},
		},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan() error: %v", err)
	}

	report := svc.ExecuteCleanup(plan, true)
	if !report.DryRun {
		t.Error("report.DryRun = false, want true")
	}
	if report.DeletedCount != 2 {
		t.Errorf("dry run DeletedCount = %d, want 2", report.DeletedCount)
	}

	// Verify no files were actually deleted
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("dry run should not delete files: %s missing", f)
		}
	}
}

func TestExecuteCleanup_RealDelete(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	createMockVideoFiles(t, folderPath, 5, 512)

	svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {
			Enabled:    true,
			CountBased: &CountPolicy{Enabled: true, MaxCount: 3},
		},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan() error: %v", err)
	}

	report := svc.ExecuteCleanup(plan, false)
	if report.DryRun {
		t.Error("report.DryRun = true, want false")
	}
	if report.DeletedCount != 2 {
		t.Errorf("DeletedCount = %d, want 2", report.DeletedCount)
	}
	if len(report.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", report.Errors)
	}

	// Verify remaining files
	entries, _ := os.ReadDir(folderPath)
	remaining := 0
	for _, e := range entries {
		if !e.IsDir() {
			remaining++
		}
	}
	if remaining != 3 {
		t.Errorf("remaining files = %d, want 3", remaining)
	}
}

func TestDetectFolders(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	tcPath := filepath.Join(partDir, "TeslaCam")

	for _, name := range []string{"SavedClips", "SentryClips", "RecentClips"} {
		if err := os.MkdirAll(filepath.Join(tcPath, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Also create a file (should be ignored)
	os.WriteFile(filepath.Join(tcPath, "somefile.txt"), []byte("x"), 0644)

	folders := svc.DetectFolders(partDir)
	if len(folders) != 3 {
		t.Errorf("DetectFolders() len = %d, want 3", len(folders))
	}

	// No TeslaCam dir
	emptyDir := t.TempDir()
	folders = svc.DetectFolders(emptyDir)
	if len(folders) != 0 {
		t.Errorf("DetectFolders() for empty = %d, want 0", len(folders))
	}
}

func TestCleanupOrphanedThumbnails(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	thumbDir := t.TempDir()

	// Create some thumbnail files
	os.WriteFile(filepath.Join(thumbDir, "abc123.png"), []byte("thumb"), 0644)
	os.WriteFile(filepath.Join(thumbDir, "def456.png"), []byte("thumb"), 0644)
	os.WriteFile(filepath.Join(thumbDir, "ghi789.png"), []byte("thumb"), 0644)
	os.WriteFile(filepath.Join(thumbDir, "readme.txt"), []byte("text"), 0644) // non-png, ignored

	// Only "abc123" still has a corresponding video
	existsFunc := func(hash string) bool {
		return hash == "abc123"
	}

	removed := svc.CleanupOrphanedThumbnails(thumbDir, existsFunc)
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}

	// Verify abc123.png still exists
	if _, err := os.Stat(filepath.Join(thumbDir, "abc123.png")); err != nil {
		t.Error("abc123.png should still exist")
	}
	// Verify readme.txt still exists (not a .png)
	if _, err := os.Stat(filepath.Join(thumbDir, "readme.txt")); err != nil {
		t.Error("readme.txt should still exist")
	}
}
