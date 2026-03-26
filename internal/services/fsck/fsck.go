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

type CheckResult struct {
	Partition string    `json:"partition"`
	StartedAt time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Status     Status    `json:"status"`
	Output     string    `json:"output,omitempty"`
	ExitCode   int       `json:"exit_code"`
	ErrorMsg   string    `json:"error,omitempty"`
}

type RunStatus struct {
	Running   bool         `json:"running"`
	Partition string       `json:"partition,omitempty"`
	StartedAt *time.Time   `json:"started_at,omitempty"`
	Results   []CheckResult `json:"results,omitempty"`
}

type Runner struct {
	cfg         *config.Config
	mu          sync.Mutex
	running     bool
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

// Start begins an fsck check on the specified partitions.
func (r *Runner) Start(partitions []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("fsck is already running")
	}

	r.running = true

	type cancelCtx struct {
		cancelled bool
		mu        sync.Mutex
	}
	ctx := &cancelCtx{}

	r.cancelFn = func() {
		ctx.mu.Lock()
		ctx.cancelled = true
		ctx.mu.Unlock()
	}

	go func() {
		for _, part := range partitions {
			ctx.mu.Lock()
			cancelled := ctx.cancelled
			ctx.mu.Unlock()
			if cancelled {
				break
			}

			result := CheckResult{
				Partition: part,
				StartedAt: time.Now(),
				Status:    StatusRunning,
			}

			r.mu.Lock()
			r.current = &result
			r.mu.Unlock()

			imgPath := r.resolveImagePath(part)
			if imgPath == "" {
				result.Status = StatusFailed
				result.ErrorMsg = "unknown partition: " + part
				result.FinishedAt = time.Now()
				result.ExitCode = -1
			} else {
				cmd := exec.Command("fsck.fat", "-n", "-v", imgPath)
				output, err := cmd.CombinedOutput()
				result.Output = string(output)
				result.FinishedAt = time.Now()

				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						result.ExitCode = exitErr.ExitCode()
					} else {
						result.ExitCode = -1
					}
					result.Status = StatusFailed
					result.ErrorMsg = err.Error()
				} else {
					result.ExitCode = 0
					result.Status = StatusDone
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
		r.mu.Unlock()
	}()

	return nil
}

// Cancel stops the current fsck operation.
func (r *Runner) Cancel() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return fmt.Errorf("no fsck operation running")
	}

	if r.cancelFn != nil {
		r.cancelFn()
	}
	r.running = false
	r.current = nil
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
