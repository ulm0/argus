package cleanup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A pinned event must survive retention, and must not show up in the preview
// counts either — otherwise the UI promises a deletion that never happens.
func TestCalculateCleanupPlan_SkipsPinnedEvents(t *testing.T) {
	cfg, _ := testConfig(t)
	svc := NewService(cfg)

	partDir := t.TempDir()
	folderPath := filepath.Join(partDir, "TeslaCam", "SavedClips")
	now := time.Now()
	var oldest string
	for i := 0; i < 5; i++ {
		when := now.Add(-time.Duration(i) * 24 * time.Hour)
		oldest = when.Format("2006-01-02_15-04-05")
		makeEventDir(t, folderPath, oldest, when)
	}

	if err := os.WriteFile(filepath.Join(folderPath, oldest, keepMarker), nil, 0644); err != nil {
		t.Fatal(err)
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

	if plan.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2 (4 unpinned events - keep last 2)", plan.TotalCount)
	}
	for _, ev := range plan.Breakdown["SavedClips"] {
		if ev.Name == oldest {
			t.Fatalf("pinned event %s scheduled for deletion", oldest)
		}
	}
}
