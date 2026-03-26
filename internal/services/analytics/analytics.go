package analytics

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ulm0/argus/internal/config"
)

type PartitionUsage struct {
	Name       string  `json:"name"`
	Label      string  `json:"label"`
	TotalBytes int64   `json:"total_bytes"`
	UsedBytes  int64   `json:"used_bytes"`
	FreeBytes  int64   `json:"free_bytes"`
	Percent    float64 `json:"percent_used"`
}

type VideoStats struct {
	Folder    string `json:"folder"`
	Count     int    `json:"count"`
	SizeBytes int64  `json:"size_bytes"`
}

type HealthStatus struct {
	Status          string   `json:"status"` // healthy, caution, warning, critical
	Score           int      `json:"score"`
	Alerts          []string `json:"alerts"`
	Recommendations []string `json:"recommendations"`
}

type FolderBreakdown struct {
	Name     string  `json:"name"`
	Count    int     `json:"count"`
	SizeMB   float64 `json:"size_mb"`
	Priority string  `json:"priority"` // high, medium, low
}

type CompleteAnalytics struct {
	PartitionUsage    []PartitionUsage  `json:"partition_usage"`
	VideoStatistics   []VideoStats      `json:"video_statistics"`
	StorageHealth     HealthStatus      `json:"storage_health"`
	RecordingEstimate map[string]any    `json:"recording_estimate"`
	FolderBreakdown   []FolderBreakdown `json:"folder_breakdown"`
	LastUpdated       string            `json:"last_updated"`
}

type Service struct {
	cfg *config.Config
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

// GetPartitionUsage returns disk usage for all mounted partitions.
func (s *Service) GetPartitionUsage() []PartitionUsage {
	var usages []PartitionUsage

	type partInfo struct {
		key   string
		label string
	}

	parts := []partInfo{
		{"part1", s.cfg.DiskImages.CamLabel},
		{"part2", s.cfg.DiskImages.LightshowLabel},
	}
	if s.cfg.DiskImages.MusicEnabled {
		parts = append(parts, partInfo{"part3", s.cfg.DiskImages.MusicLabel})
	}

	for _, p := range parts {
		for _, ro := range []bool{true, false} {
			mountPath := s.cfg.MountPath(p.key, ro)
			usage := getDiskUsage(mountPath)
			if usage.TotalBytes > 0 {
				usage.Name = p.key
				usage.Label = p.label
				usages = append(usages, usage)
				break
			}
		}
	}

	return usages
}

// GetVideoStatistics returns video counts and sizes per TeslaCam folder.
func (s *Service) GetVideoStatistics() []VideoStats {
	var stats []VideoStats

	// Find TeslaCam path
	for _, ro := range []bool{true, false} {
		base := s.cfg.MountPath("part1", ro)
		tcPath := filepath.Join(base, "TeslaCam")
		if !isDir(tcPath) {
			continue
		}

		entries, err := os.ReadDir(tcPath)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			folderPath := filepath.Join(tcPath, e.Name())
			count, size := countAndSizeVideos(folderPath)
			stats = append(stats, VideoStats{
				Folder:    e.Name(),
				Count:     count,
				SizeBytes: size,
			})
		}
		break
	}

	return stats
}

// GetStorageHealth assesses overall storage health.
func (s *Service) GetStorageHealth() HealthStatus {
	usages := s.GetPartitionUsage()

	health := HealthStatus{
		Status: "healthy",
		Score:  100,
	}

	for _, u := range usages {
		if u.Percent > 95 {
			health.Status = "critical"
			health.Score -= 40
			health.Alerts = append(health.Alerts, u.Label+" is critically full (>95%)")
			health.Recommendations = append(health.Recommendations, "Run cleanup on "+u.Label+" immediately")
		} else if u.Percent > 85 {
			if health.Status == "healthy" {
				health.Status = "warning"
			}
			health.Score -= 20
			health.Alerts = append(health.Alerts, u.Label+" is nearly full (>85%)")
			health.Recommendations = append(health.Recommendations, "Consider running cleanup on "+u.Label)
		} else if u.Percent > 70 {
			if health.Status == "healthy" {
				health.Status = "caution"
			}
			health.Score -= 10
		}
	}

	if health.Score < 0 {
		health.Score = 0
	}

	return health
}

// GetCompleteAnalytics aggregates all analytics data.
func (s *Service) GetCompleteAnalytics() CompleteAnalytics {
	usages := s.GetPartitionUsage()
	videoStats := s.GetVideoStatistics()
	health := s.GetStorageHealth()

	var breakdowns []FolderBreakdown
	for _, vs := range videoStats {
		priority := "low"
		if vs.Folder == "SentryClips" || vs.Folder == "SavedClips" {
			priority = "high"
		} else if vs.Folder == "RecentClips" {
			priority = "medium"
		}
		breakdowns = append(breakdowns, FolderBreakdown{
			Name:     vs.Folder,
			Count:    vs.Count,
			SizeMB:   float64(vs.SizeBytes) / (1024 * 1024),
			Priority: priority,
		})
	}

	// Estimate recording time
	estimate := map[string]any{}
	for _, u := range usages {
		if u.Name == "part1" && u.FreeBytes > 0 {
			// ~25 MB/min for 6 cameras
			minutesLeft := float64(u.FreeBytes) / (25 * 1024 * 1024)
			estimate["minutes_remaining"] = minutesLeft
			estimate["hours_remaining"] = minutesLeft / 60
		}
	}

	return CompleteAnalytics{
		PartitionUsage:    usages,
		VideoStatistics:   videoStats,
		StorageHealth:     health,
		RecordingEstimate: estimate,
		FolderBreakdown:   breakdowns,
	}
}

func getDiskUsage(path string) PartitionUsage {
	var used int64
	filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			used += info.Size()
		}
		return nil
	})
	// This is approximate; real implementation uses syscall.Statfs on Linux
	return PartitionUsage{
		UsedBytes: used,
	}
}

func countAndSizeVideos(dir string) (int, int64) {
	count := 0
	var size int64

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext == ".mp4" || ext == ".avi" || ext == ".mov" {
			count++
			size += info.Size()
		}
		return nil
	})

	return count, size
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
