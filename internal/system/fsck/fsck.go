package fsck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
)

type Mode string

const (
	ModeQuick  Mode = "quick"
	ModeRepair Mode = "repair"
)

type Status struct {
	Running    bool      `json:"running"`
	Partition  int       `json:"partition,omitempty"`
	Mode       Mode      `json:"mode,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	Progress   string    `json:"progress,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type HistoryEntry struct {
	Partition  int       `json:"partition"`
	Mode       Mode      `json:"mode"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Duration   float64   `json:"duration_seconds"`
	ExitCode   int       `json:"exit_code"`
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
}

type Runner struct {
	cfg     *config.Config
	mu      sync.Mutex
	status  Status
	cancel  context.CancelFunc
	history []HistoryEntry
}

func NewRunner(cfg *config.Config) *Runner {
	r := &Runner{cfg: cfg}
	r.loadHistory()
	return r
}

func (r *Runner) Start(partition int, mode Mode) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status.Running {
		return fmt.Errorf("fsck already running on partition %d", r.status.Partition)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.status = Status{
		Running:   true,
		Partition: partition,
		Mode:      mode,
		StartedAt: time.Now(),
	}
	r.saveStatus()

	go r.runBackground(ctx, partition, mode)
	return nil
}

func (r *Runner) Cancel() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.status.Running {
		return fmt.Errorf("no fsck running")
	}
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}

func (r *Runner) GetStatus() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Runner) GetHistory() []HistoryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.history
}

func (r *Runner) GetLastCheck(partition int) *HistoryEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := len(r.history) - 1; i >= 0; i-- {
		if r.history[i].Partition == partition {
			entry := r.history[i]
			return &entry
		}
	}
	return nil
}

func (r *Runner) runBackground(ctx context.Context, partition int, mode Mode) {
	started := time.Now()
	var exitCode int
	var fsckErr string

	defer func() {
		r.mu.Lock()
		entry := HistoryEntry{
			Partition:  partition,
			Mode:       mode,
			StartedAt:  started,
			FinishedAt: time.Now(),
			Duration:   time.Since(started).Seconds(),
			ExitCode:   exitCode,
			Success:    exitCode == 0,
			Error:      fsckErr,
		}
		r.history = append(r.history, entry)
		if len(r.history) > 50 {
			r.history = r.history[len(r.history)-50:]
		}
		r.status = Status{Running: false}
		r.saveStatus()
		r.saveHistory()
		r.mu.Unlock()
	}()

	imgPath := r.getImagePath(partition)
	if imgPath == "" {
		fsckErr = "unknown partition"
		exitCode = 1
		return
	}

	// Determine filesystem type based on partition
	fsckCmd := "fsck.vfat"
	if partition == 1 {
		fsckCmd = "fsck.exfat"
	}

	args := []string{}
	switch mode {
	case ModeQuick:
		args = append(args, "-n") // read-only check
	case ModeRepair:
		args = append(args, "-a") // auto-repair
	}
	args = append(args, imgPath)

	cmd := exec.CommandContext(ctx, fsckCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		fsckErr = string(output)
	}

	logger.L.WithField("partition", partition).WithField("mode", mode).WithField("exit_code", exitCode).Info("fsck complete")
}

func (r *Runner) getImagePath(partition int) string {
	switch partition {
	case 1:
		return r.cfg.ImgCamPath
	case 2:
		return r.cfg.ImgLightshow
	case 3:
		return r.cfg.ImgMusicPath
	default:
		return ""
	}
}

func (r *Runner) saveStatus() {
	path := filepath.Join(r.cfg.GadgetDir, "fsck_status.json")
	data, _ := json.Marshal(r.status)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	os.Rename(tmp, path)
}

func (r *Runner) loadHistory() {
	path := filepath.Join(r.cfg.GadgetDir, "fsck_history.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &r.history)
}

func (r *Runner) saveHistory() {
	path := filepath.Join(r.cfg.GadgetDir, "fsck_history.json")
	data, _ := json.Marshal(r.history)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	os.Rename(tmp, path)
}
