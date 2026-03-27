package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	repo       = "ulm0/argus"
	apiBase    = "https://api.github.com"
	binaryDest = "/usr/local/bin/argus"
)

// Release holds information about a GitHub release.
type Release struct {
	Version     string
	DownloadURL string
	PublishedAt time.Time
}

var (
	pendingMu      sync.RWMutex
	pendingRelease *Release
)

// SetPendingRelease stores a detected pending release for the API handler to read.
func SetPendingRelease(r *Release) {
	pendingMu.Lock()
	defer pendingMu.Unlock()
	pendingRelease = r
}

// GetPendingRelease returns the currently stored pending release, or nil if none.
func GetPendingRelease() *Release {
	pendingMu.RLock()
	defer pendingMu.RUnlock()
	return pendingRelease
}

// IsOnline reports whether api.github.com is reachable.
func IsOnline() bool {
	conn, err := net.DialTimeout("tcp", "api.github.com:443", 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// CheckLatest fetches the latest GitHub release and compares it to currentVersion.
// Returns nil, nil if already up to date.
func CheckLatest(currentVersion string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "argus/"+currentVersion)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var gr githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	latest := strings.TrimPrefix(gr.TagName, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	if current == "dev" {
		return nil, nil
	}
	if compareSemver(latest, current) <= 0 {
		return nil, nil
	}

	assetName := assetNameForArch(gr.TagName)
	downloadURL := ""
	for _, a := range gr.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("no asset found for %s (looked for %s)", gr.TagName, assetName)
	}

	return &Release{
		Version:     gr.TagName,
		DownloadURL: downloadURL,
		PublishedAt: gr.PublishedAt,
	}, nil
}

// Install downloads the raw argus binary published by GoReleaser (binary format,
// no archive wrapping), atomically replaces /usr/local/bin/argus, and restarts
// the systemd service.
//
// The temp file is created in the same directory as binaryDest so that
// os.Rename stays on the same filesystem (avoiding EXDEV cross-device errors).
func Install(release *Release) error {
	destDir := binaryDest[:strings.LastIndex(binaryDest, "/")]

	tmp, err := os.CreateTemp(destDir, ".argus-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := downloadTo(release.DownloadURL, tmp); err != nil {
		tmp.Close()
		return fmt.Errorf("download: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync download: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod binary: %w", err)
	}

	// Back up the existing binary before swapping.
	if _, err := os.Stat(binaryDest); err == nil {
		_ = os.Rename(binaryDest, binaryDest+".backup")
	}

	if err := os.Rename(tmpPath, binaryDest); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	// Restart service; best-effort — the binary is already replaced.
	_ = exec.Command("systemctl", "restart", "argus.service").Run()

	return nil
}

// assetNameForArch returns the GoReleaser binary asset filename for the current
// architecture, matching the name_template in .goreleaser.yaml.
// e.g. "argus_1.2.3_linux_arm64", "argus_1.2.3_linux_armv6"
func assetNameForArch(tag string) string {
	version := strings.TrimPrefix(tag, "v")
	var archSuffix string
	switch runtime.GOARCH {
	case "arm64":
		archSuffix = "arm64"
	case "arm":
		if goarm == "7" {
			archSuffix = "armv7"
		} else {
			archSuffix = "armv6"
		}
	case "amd64":
		archSuffix = "amd64"
	default:
		archSuffix = runtime.GOARCH
	}
	return fmt.Sprintf("argus_%s_linux_%s", version, archSuffix)
}

func downloadTo(url string, dst *os.File) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}

// compareSemver compares two semver strings (e.g. "1.2.3").
// Returns 1 if a > b, -1 if a < b, 0 if equal.
// Non-numeric segments are treated as 0.
func compareSemver(a, b string) int {
	partsA := strings.SplitN(a, ".", 3)
	partsB := strings.SplitN(b, ".", 3)
	for len(partsA) < 3 {
		partsA = append(partsA, "0")
	}
	for len(partsB) < 3 {
		partsB = append(partsB, "0")
	}
	for i := 0; i < 3; i++ {
		na, _ := strconv.Atoi(partsA[i])
		nb, _ := strconv.Atoi(partsB[i])
		if na > nb {
			return 1
		}
		if na < nb {
			return -1
		}
	}
	return 0
}
