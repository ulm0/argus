package cleanup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ulm0/argus/internal/config"
)

type PolicyType string

const (
	PolicyAge   PolicyType = "age"
	PolicySize  PolicyType = "size"
	PolicyCount PolicyType = "count"
)

type FolderPolicy struct {
	Enabled      bool `json:"enabled"`
	BootCleanup  bool `json:"boot_cleanup"`
	AgeBased     *AgePolicy   `json:"age_based,omitempty"`
	SizeBased    *SizePolicy  `json:"size_based,omitempty"`
	CountBased   *CountPolicy `json:"count_based,omitempty"`
}

type AgePolicy struct {
	Enabled  bool `json:"enabled"`
	MaxDays  int  `json:"max_days"`
}

type SizePolicy struct {
	Enabled bool    `json:"enabled"`
	MaxGB   float64 `json:"max_gb"`
}

type CountPolicy struct {
	Enabled  bool `json:"enabled"`
	MaxCount int  `json:"max_count"`
}

type CleanupPlan struct {
	TotalCount int                        `json:"total_count"`
	TotalSize  int64                      `json:"total_size"`
	Breakdown  map[string][]FileToDelete  `json:"breakdown_by_folder"`
}

type FileToDelete struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Reason   string    `json:"reason"`
}

type CleanupReport struct {
	DryRun       bool    `json:"dry_run"`
	DeletedCount int     `json:"deleted_count"`
	DeletedSize  float64 `json:"deleted_size_gb"`
	Errors       []string `json:"errors,omitempty"`
}

type Service struct {
	cfg        *config.Config
	policies   map[string]FolderPolicy
	configFile string
}

func NewService(cfg *config.Config) *Service {
	s := &Service{
		cfg:        cfg,
		policies:   make(map[string]FolderPolicy),
		configFile: filepath.Join(cfg.GadgetDir, "cleanup_config.json"),
	}
	s.loadPolicies()
	return s
}

func (s *Service) loadPolicies() {
	data, err := os.ReadFile(s.configFile)
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.policies)
}

// SavePolicies persists cleanup policies to disk atomically.
// In-memory state is only updated after a successful disk write.
func (s *Service) SavePolicies(policies map[string]FolderPolicy) error {
	data, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.configFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.configFile); err != nil {
		os.Remove(tmp)
		return err
	}
	s.policies = policies
	return nil
}

// GetPolicies returns the current cleanup policies.
func (s *Service) GetPolicies() map[string]FolderPolicy {
	return s.policies
}

// DetectFolders finds TeslaCam subfolders on the partition.
func (s *Service) DetectFolders(partitionPath string) []string {
	tcPath := filepath.Join(partitionPath, "TeslaCam")
	entries, err := os.ReadDir(tcPath)
	if err != nil {
		return nil
	}

	var folders []string
	for _, e := range entries {
		if e.IsDir() {
			folders = append(folders, e.Name())
		}
	}
	return folders
}

// GetPoliciesForDetectedFolders returns policies merged with detected folders.
func (s *Service) GetPoliciesForDetectedFolders(partitionPath string) map[string]FolderPolicy {
	folders := s.DetectFolders(partitionPath)
	result := make(map[string]FolderPolicy)

	for _, folder := range folders {
		if p, ok := s.policies[folder]; ok {
			result[folder] = p
		} else {
			result[folder] = FolderPolicy{}
		}
	}
	return result
}

// CalculateCleanupPlan builds a deletion plan based on the configured policies.
func (s *Service) CalculateCleanupPlan(partitionPath string) (*CleanupPlan, error) {
	tcPath := filepath.Join(partitionPath, "TeslaCam")
	plan := &CleanupPlan{
		Breakdown: make(map[string][]FileToDelete),
	}

	for folder, policy := range s.policies {
		if !policy.Enabled {
			continue
		}

		folderPath := filepath.Join(tcPath, folder)
		if _, err := os.Stat(folderPath); err != nil {
			continue
		}

		files := s.collectVideoFiles(folderPath)
		var toDelete []FileToDelete

		if policy.AgeBased != nil && policy.AgeBased.Enabled {
			cutoff := time.Now().Add(-time.Duration(policy.AgeBased.MaxDays) * 24 * time.Hour)
			for _, f := range files {
				if f.Modified.Before(cutoff) {
					f.Reason = fmt.Sprintf("older than %d days", policy.AgeBased.MaxDays)
					toDelete = append(toDelete, f)
				}
			}
		}

		if policy.SizeBased != nil && policy.SizeBased.Enabled {
			maxBytes := int64(policy.SizeBased.MaxGB * 1024 * 1024 * 1024)
			var totalSize int64
			for _, f := range files {
				totalSize += f.Size
			}

			if totalSize > maxBytes {
				// Sort oldest first, delete until under limit
				sorted := make([]FileToDelete, len(files))
				copy(sorted, files)
				sort.Slice(sorted, func(i, j int) bool {
					return sorted[i].Modified.Before(sorted[j].Modified)
				})

				for _, f := range sorted {
					if totalSize <= maxBytes {
						break
					}
					f.Reason = fmt.Sprintf("folder exceeds %.1f GB", policy.SizeBased.MaxGB)
					toDelete = append(toDelete, f)
					totalSize -= f.Size
				}
			}
		}

		if policy.CountBased != nil && policy.CountBased.Enabled && len(files) > policy.CountBased.MaxCount {
			sorted := make([]FileToDelete, len(files))
			copy(sorted, files)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].Modified.Before(sorted[j].Modified)
			})

			excess := len(files) - policy.CountBased.MaxCount
			for i := 0; i < excess; i++ {
				sorted[i].Reason = fmt.Sprintf("exceeds max count of %d", policy.CountBased.MaxCount)
				toDelete = append(toDelete, sorted[i])
			}
		}

		// Deduplicate
		seen := make(map[string]bool)
		var unique []FileToDelete
		for _, f := range toDelete {
			if !seen[f.Path] {
				seen[f.Path] = true
				unique = append(unique, f)
				plan.TotalCount++
				plan.TotalSize += f.Size
			}
		}

		if len(unique) > 0 {
			plan.Breakdown[folder] = unique
		}
	}

	return plan, nil
}

// ExecuteCleanup deletes files according to the plan.
func (s *Service) ExecuteCleanup(plan *CleanupPlan, dryRun bool) CleanupReport {
	report := CleanupReport{DryRun: dryRun}

	for _, files := range plan.Breakdown {
		for _, f := range files {
			if dryRun {
				report.DeletedCount++
				report.DeletedSize += float64(f.Size) / (1024 * 1024 * 1024)
				continue
			}

			if err := os.Remove(f.Path); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("delete %s: %v", f.Name, err))
			} else {
				report.DeletedCount++
				report.DeletedSize += float64(f.Size) / (1024 * 1024 * 1024)
			}
		}
	}

	return report
}

// CleanupOrphanedThumbnails removes thumbnails that no longer have corresponding videos.
func (s *Service) CleanupOrphanedThumbnails(thumbnailDir string, videoPathsExist func(string) bool) int {
	entries, err := os.ReadDir(thumbnailDir)
	if err != nil {
		return 0
	}

	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		hash := strings.TrimSuffix(e.Name(), ".png")
		if !videoPathsExist(hash) {
			os.Remove(filepath.Join(thumbnailDir, e.Name()))
			removed++
		}
	}
	return removed
}

func (s *Service) collectVideoFiles(folderPath string) []FileToDelete {
	var files []FileToDelete

	filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext == ".mp4" || ext == ".avi" || ext == ".mov" {
			files = append(files, FileToDelete{
				Path:     path,
				Name:     info.Name(),
				Size:     info.Size(),
				Modified: info.ModTime(),
			})
		}
		return nil
	})

	// Sort newest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].Modified.After(files[j].Modified)
	})

	return files
}
