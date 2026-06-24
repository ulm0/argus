package cleanup

import (
	"encoding/json"
	"fmt"
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

// makeEventDir creates a SavedClips/SentryClips-style event directory with the
// six camera mp4s + event.json + thumb.png that Tesla writes.
func makeEventDir(t *testing.T, parent, name string, modTime time.Time) {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	files := []string{
		name + "-front.mp4",
		name + "-back.mp4",
		name + "-left_repeater.mp4",
		name + "-right_repeater.mp4",
		"event.json",
		"thumb.png",
	}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(dir, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

// makeSessionFiles creates RecentClips-style session files (one per camera).
func makeSessionFiles(t *testing.T, parent, session string, modTime time.Time) {
	t.Helper()
	if err := os.MkdirAll(parent, 0755); err != nil {
		t.Fatal(err)
	}
	cameras := []string{"front", "back", "left_repeater", "right_repeater"}
	for _, cam := range cameras {
		path := filepath.Join(parent, fmt.Sprintf("%s-%s.mp4", session, cam))
		if err := os.WriteFile(path, []byte("y"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSavePoliciesRejectsTraversalKeys(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	for _, key := range []string{"../etc", "../../home/user", "..", ".", "a/b", `a\b`, ""} {
		err := svc.SavePolicies(map[string]FolderPolicy{key: {Enabled: true, KeepLast: 1}})
		if err == nil {
			t.Errorf("SavePolicies accepted unsafe folder key %q", key)
		}
	}
	// A normal folder name is still accepted.
	if err := svc.SavePolicies(map[string]FolderPolicy{"SentryClips": {Enabled: true, KeepLast: 5}}); err != nil {
		t.Errorf("SavePolicies rejected a valid key: %v", err)
	}
}

func TestSaveAndLoadPolicies(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	if got := len(svc.GetPolicies()); got != 0 {
		t.Fatalf("initial policies len = %d, want 0", got)
	}

	newPolicies := map[string]FolderPolicy{
		"SavedClips":  {Enabled: true, BootCleanup: true, KeepLast: 30},
		"SentryClips": {Enabled: true, KeepLast: 10},
	}
	if err := svc.SavePolicies(newPolicies); err != nil {
		t.Fatalf("SavePolicies() error: %v", err)
	}

	data, err := os.ReadFile(svc.configFile)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	var loaded map[string]FolderPolicy
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal saved policies: %v", err)
	}
	if loaded["SavedClips"].KeepLast != 30 {
		t.Errorf("SavedClips KeepLast = %d, want 30", loaded["SavedClips"].KeepLast)
	}

	// Reload and confirm it survives.
	svc2 := NewService(cfg)
	if got := svc2.GetPolicies()["SentryClips"].KeepLast; got != 10 {
		t.Errorf("reloaded SentryClips KeepLast = %d, want 10", got)
	}
}

func TestLoadLegacyPolicies(t *testing.T) {
	cfg, _ := testConfig(t)
	// Write a config in the pre-simplification shape and verify it migrates.
	legacy := []byte(`{
  "SavedClips": {
    "enabled": true,
    "boot_cleanup": true,
    "count_based": {"enabled": true, "max_count": 25}
  },
  "SentryClips": {
    "enabled": true,
    "age_based": {"enabled": true, "max_days": 30}
  }
}`)
	cfgFile := filepath.Join(cfg.GadgetDir, "cleanup_config.json")
	if err := os.WriteFile(cfgFile, legacy, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewService(cfg)
	got := svc.GetPolicies()
	if got["SavedClips"].KeepLast != 25 {
		t.Errorf("SavedClips KeepLast (legacy count_based) = %d, want 25", got["SavedClips"].KeepLast)
	}
	// SentryClips had no count info → falls back to default.
	if got["SentryClips"].KeepLast != defaultKeepLast {
		t.Errorf("SentryClips KeepLast (no count info) = %d, want %d", got["SentryClips"].KeepLast, defaultKeepLast)
	}
}

func TestCalculateCleanupPlan_EventDirs(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	now := time.Now()
	for i := 0; i < 5; i++ {
		ts := now.Add(-time.Duration(i) * 24 * time.Hour).Format("2006-01-02_15-04-05")
		makeEventDir(t, folderPath, ts, now.Add(-time.Duration(i)*24*time.Hour))
	}

	if err := svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {Enabled: true, KeepLast: 2},
	}); err != nil {
		t.Fatal(err)
	}

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan: %v", err)
	}

	if plan.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3 (5 events - keep last 2)", plan.TotalCount)
	}
	events := plan.Breakdown["SavedClips"]
	if len(events) != 3 {
		t.Fatalf("SavedClips events to delete = %d, want 3", len(events))
	}
	// Sanity: each event keeps its 6 files together.
	for _, ev := range events {
		if len(ev.Files) != 6 {
			t.Errorf("event %s has %d files, want 6 (cameras + metadata)", ev.Name, len(ev.Files))
		}
	}
}

func TestCalculateCleanupPlan_Sessions(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "RecentClips")
	now := time.Now()
	for i := 0; i < 4; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format("2006-01-02_15-04-05")
		makeSessionFiles(t, folderPath, ts, now.Add(-time.Duration(i)*time.Hour))
	}

	if err := svc.SavePolicies(map[string]FolderPolicy{
		"RecentClips": {Enabled: true, KeepLast: 1},
	}); err != nil {
		t.Fatal(err)
	}

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan: %v", err)
	}

	if plan.TotalCount != 3 {
		t.Errorf("TotalCount = %d, want 3 (4 sessions - keep last 1)", plan.TotalCount)
	}
	for _, ev := range plan.Breakdown["RecentClips"] {
		if len(ev.Files) != 4 {
			t.Errorf("session %s has %d files, want 4 cameras", ev.Name, len(ev.Files))
		}
	}
}

func TestCalculateCleanupPlan_NoOpUnderLimit(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	now := time.Now()
	makeEventDir(t, folderPath, "2024-01-01_00-00-00", now)
	makeEventDir(t, folderPath, "2024-01-02_00-00-00", now)

	svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {Enabled: true, KeepLast: 5},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan: %v", err)
	}
	if plan.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0 (under limit)", plan.TotalCount)
	}
}

func TestCalculateCleanupPlan_DisabledPolicy(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	now := time.Now()
	for i := 0; i < 3; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format("2006-01-02_15-04-05")
		makeEventDir(t, folderPath, ts, now.Add(-time.Duration(i)*time.Hour))
	}

	svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {Enabled: false, KeepLast: 1},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatalf("CalculateCleanupPlan: %v", err)
	}
	if plan.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0 (policy disabled)", plan.TotalCount)
	}
}

func TestExecuteCleanup_DryRun(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	now := time.Now()
	for i := 0; i < 4; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format("2006-01-02_15-04-05")
		makeEventDir(t, folderPath, ts, now.Add(-time.Duration(i)*time.Hour))
	}

	svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {Enabled: true, KeepLast: 2},
	})

	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatal(err)
	}
	report := svc.ExecuteCleanup(plan, true)
	if !report.DryRun {
		t.Error("DryRun = false, want true")
	}
	if report.DeletedCount != 2 {
		t.Errorf("dry run DeletedCount = %d, want 2", report.DeletedCount)
	}
	// Ensure dry run leaves disk untouched.
	entries, _ := os.ReadDir(folderPath)
	if len(entries) != 4 {
		t.Errorf("dry run modified disk: %d events remain, want 4", len(entries))
	}
}

func TestExecuteCleanup_RealDelete_EventDirs(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	now := time.Now()
	for i := 0; i < 4; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format("2006-01-02_15-04-05")
		makeEventDir(t, folderPath, ts, now.Add(-time.Duration(i)*time.Hour))
	}

	svc.SavePolicies(map[string]FolderPolicy{
		"SavedClips": {Enabled: true, KeepLast: 2},
	})
	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatal(err)
	}
	report := svc.ExecuteCleanup(plan, false)
	if len(report.Errors) != 0 {
		t.Errorf("errors = %v, want none", report.Errors)
	}
	if report.DeletedCount != 2 {
		t.Errorf("DeletedCount = %d, want 2", report.DeletedCount)
	}

	entries, _ := os.ReadDir(folderPath)
	dirs := 0
	for _, e := range entries {
		if e.IsDir() {
			dirs++
		}
	}
	if dirs != 2 {
		t.Errorf("remaining event dirs = %d, want 2", dirs)
	}
}

func TestExecuteCleanup_RealDelete_Sessions(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "RecentClips")
	now := time.Now()
	for i := 0; i < 3; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour).Format("2006-01-02_15-04-05")
		makeSessionFiles(t, folderPath, ts, now.Add(-time.Duration(i)*time.Hour))
	}

	svc.SavePolicies(map[string]FolderPolicy{
		"RecentClips": {Enabled: true, KeepLast: 1},
	})
	plan, err := svc.CalculateCleanupPlan(partDir)
	if err != nil {
		t.Fatal(err)
	}
	report := svc.ExecuteCleanup(plan, false)
	if len(report.Errors) != 0 {
		t.Errorf("errors = %v, want none", report.Errors)
	}
	if report.DeletedCount != 2 {
		t.Errorf("DeletedCount = %d, want 2", report.DeletedCount)
	}
	entries, _ := os.ReadDir(folderPath)
	if len(entries) != 4 { // 1 session × 4 cameras
		t.Errorf("remaining files = %d, want 4 (newest session intact)", len(entries))
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
	os.WriteFile(filepath.Join(tcPath, "stray.txt"), []byte("x"), 0644)

	folders := svc.DetectFolders(partDir)
	if len(folders) != 3 {
		t.Errorf("DetectFolders len = %d, want 3", len(folders))
	}
}

func TestGetPoliciesForDetectedFolders_DefaultsKeepLast(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	tcPath := filepath.Join(partDir, "TeslaCam")
	if err := os.MkdirAll(filepath.Join(tcPath, "SavedClips"), 0755); err != nil {
		t.Fatal(err)
	}

	got := svc.GetPoliciesForDetectedFolders(partDir)
	if got["SavedClips"].KeepLast != defaultKeepLast {
		t.Errorf("SavedClips default KeepLast = %d, want %d", got["SavedClips"].KeepLast, defaultKeepLast)
	}
}

func TestCleanupOrphanedThumbnails(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	thumbDir := t.TempDir()
	os.WriteFile(filepath.Join(thumbDir, "abc123.png"), []byte("thumb"), 0644)
	os.WriteFile(filepath.Join(thumbDir, "def456.png"), []byte("thumb"), 0644)
	os.WriteFile(filepath.Join(thumbDir, "readme.txt"), []byte("text"), 0644)

	existsFunc := func(hash string) bool { return hash == "abc123" }

	removed := svc.CleanupOrphanedThumbnails(thumbDir, existsFunc)
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(thumbDir, "abc123.png")); err != nil {
		t.Error("abc123.png should still exist")
	}
	if _, err := os.Stat(filepath.Join(thumbDir, "readme.txt")); err != nil {
		t.Error("readme.txt should still exist")
	}
}
