package fsck

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
)

type Status string

const (
	StatusIdle    Status = "idle"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

type Mode string

const (
	ModeQuick  Mode = "quick"
	ModeRepair Mode = "repair"
)

type CheckResult struct {
	Partition  string    `json:"partition"`
	Mode       Mode      `json:"mode,omitempty"`
	FSType     string    `json:"fs_type,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Status     Status    `json:"status"`
	Output     string    `json:"output,omitempty"`
	ExitCode   int       `json:"exit_code"`
	ErrorMsg   string    `json:"error,omitempty"`
}

type RunStatus struct {
	Running   bool          `json:"running"`
	Partition string        `json:"partition,omitempty"`
	StartedAt *time.Time    `json:"started_at,omitempty"`
	Results   []CheckResult `json:"results,omitempty"`
}

type Runner struct {
	cfg         *config.Config
	mu          sync.Mutex
	running     bool
	currentCmd  *exec.Cmd
	cancelFn    func()
	current     *CheckResult
	history     []CheckResult
	historyFile string
}

func NewRunner(cfg *config.Config) *Runner {
	r := &Runner{
		cfg:         cfg,
		historyFile: filepath.Join(cfg.GadgetDir, "fsck_history.json"),
	}
	r.loadHistory()
	return r
}

func (r *Runner) loadHistory() {
	data, err := os.ReadFile(r.historyFile)
	if err != nil {
		return
	}
	json.Unmarshal(data, &r.history)
}

func (r *Runner) saveHistory() {
	data, _ := json.MarshalIndent(r.history, "", "  ")
	os.WriteFile(r.historyFile, data, 0644)
}

// Start begins an fsck check on the specified partitions in the requested mode.
// Empty mode is treated as ModeQuick (read-only check).
func (r *Runner) Start(partitions []string, mode Mode) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("fsck is already running")
	}

	if mode != ModeQuick && mode != ModeRepair {
		mode = ModeQuick
	}

	r.running = true

	cancelled := make(chan struct{})
	r.cancelFn = sync.OnceFunc(func() { close(cancelled) })

	go func() {
		for _, part := range partitions {
			select {
			case <-cancelled:
				r.mu.Lock()
				r.running = false
				r.current = nil
				r.cancelFn = nil
				r.currentCmd = nil
				r.mu.Unlock()
				return
			default:
			}

			result := CheckResult{
				Partition: part,
				Mode:      mode,
				StartedAt: time.Now(),
				Status:    StatusRunning,
			}

			r.mu.Lock()
			r.current = &result
			r.mu.Unlock()

			imgPath := r.resolveImagePath(part)
			if imgPath == "" {
				result.Status = StatusFailed
				result.ErrorMsg = "unknown or disabled partition: " + part
				result.FinishedAt = time.Now()
				result.ExitCode = -1
			} else {
				fsType := r.resolveFSType(part)
				bin, args, err := buildFsckCommand(fsType, mode, imgPath)
				result.FSType = fsType
				if err != nil {
					result.Status = StatusFailed
					result.ErrorMsg = err.Error()
					result.FinishedAt = time.Now()
					result.ExitCode = -1
				} else {
					cmd := exec.Command(bin, args...)

					r.mu.Lock()
					r.currentCmd = cmd
					r.mu.Unlock()

					output, runErr := cmd.CombinedOutput()

					r.mu.Lock()
					r.currentCmd = nil
					r.mu.Unlock()

					result.Output = string(output)
					result.FinishedAt = time.Now()

					if runErr != nil {
						if exitErr, ok := runErr.(*exec.ExitError); ok {
							result.ExitCode = exitErr.ExitCode()
						} else {
							result.ExitCode = -1
						}
						result.Status = StatusFailed
						result.ErrorMsg = runErr.Error()
					} else {
						result.ExitCode = 0
						result.Status = StatusDone
					}
				}
			}

			r.mu.Lock()
			r.history = append(r.history, result)
			r.saveHistory()
			r.mu.Unlock()
		}

		r.mu.Lock()
		r.running = false
		r.current = nil
		r.cancelFn = nil
		r.currentCmd = nil
		r.mu.Unlock()
	}()

	return nil
}

// Cancel stops the current fsck operation, killing the running process if active.
func (r *Runner) Cancel() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return fmt.Errorf("no fsck operation running")
	}

	if r.cancelFn != nil {
		r.cancelFn()
	}
	if r.currentCmd != nil && r.currentCmd.Process != nil {
		r.currentCmd.Process.Kill()
	}
	// Do NOT clear r.running here. Only the worker goroutine may transition
	// running→false (in its cancelled/cleanup block); clearing it from Cancel
	// while the worker is still alive lets a racing Start() pass the guard and
	// spawn a second worker that clobbers the first one's state.
	return nil
}

// GetStatus returns the current fsck status.
func (r *Runner) GetStatus() RunStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	status := RunStatus{
		Running: r.running,
	}
	if r.current != nil {
		status.Partition = r.current.Partition
		status.StartedAt = &r.current.StartedAt
	}
	return status
}

// GetHistory returns all past fsck results.
func (r *Runner) GetHistory() []CheckResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]CheckResult, len(r.history))
	copy(result, r.history)
	return result
}

// GetLastCheck returns the most recent check result for a partition.
func (r *Runner) GetLastCheck(partition string) *CheckResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := len(r.history) - 1; i >= 0; i-- {
		if r.history[i].Partition == partition {
			result := r.history[i]
			return &result
		}
	}
	return nil
}

func (r *Runner) resolveImagePath(partition string) string {
	switch partition {
	case "part1":
		return r.cfg.ImgCamPath
	case "part2":
		return r.cfg.ImgLightshow
	case "part3":
		if r.cfg.DiskImages.MusicEnabled {
			return r.cfg.ImgMusicPath
		}
		return ""
	default:
		return ""
	}
}

// resolveFSType returns the on-disk filesystem type for a partition, matching
// the layout produced by `argus setup` (see cmd/argus/cmd/setup.go).
//   - part1 (TeslaCam)         → exfat (videos may exceed 4 GB, FAT32 cannot hold them)
//   - part2 (LightShow/Chimes) → vfat
//   - part3 (Music)            → cfg.DiskImages.MusicFS (defaults to fat32 → vfat)
func (r *Runner) resolveFSType(partition string) string {
	switch partition {
	case "part1":
		return "exfat"
	case "part2":
		return "vfat"
	case "part3":
		if r.cfg.DiskImages.MusicFS == "exfat" {
			return "exfat"
		}
		return "vfat"
	default:
		return ""
	}
}

// buildFsckCommand picks the right fsck binary and flags for a (fsType, mode) pair.
// Quick mode is read-only (-n); Repair applies fixes non-interactively (-y / -a).
func buildFsckCommand(fsType string, mode Mode, imgPath string) (string, []string, error) {
	switch fsType {
	case "exfat":
		// exfatprogs: -n no-action (check only), -y repair without prompts.
		if mode == ModeRepair {
			return "fsck.exfat", []string{"-y", imgPath}, nil
		}
		return "fsck.exfat", []string{"-n", imgPath}, nil
	case "vfat", "fat", "fat32", "fat16", "fat12":
		// dosfstools: -n no-action, -a auto-repair without prompts. -v verbose.
		if mode == ModeRepair {
			return "fsck.fat", []string{"-a", "-v", imgPath}, nil
		}
		return "fsck.fat", []string{"-n", "-v", imgPath}, nil
	default:
		return "", nil, fmt.Errorf("no fsck handler for filesystem type %q", fsType)
	}
}
