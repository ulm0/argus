package updater

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

	if latest == current || current == "dev" {
		return nil, nil
	}

	assetName := archiveNameForArch(gr.TagName)
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

// Install downloads the release archive, extracts the binary, atomically replaces
// /usr/local/bin/argus, and restarts the systemd service.
func Install(release *Release) error {
	tmp, err := os.MkdirTemp("", "argus-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	archivePath := filepath.Join(tmp, "argus.tar.gz")
	if err := download(release.DownloadURL, archivePath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	binaryTmp := filepath.Join(tmp, "argus")
	if err := extractBinary(archivePath, binaryTmp); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	// Backup existing binary before swap
	if _, err := os.Stat(binaryDest); err == nil {
		_ = os.Rename(binaryDest, binaryDest+".backup")
	}

	// Atomic replace: write to .new then rename
	newPath := binaryDest + ".new"
	if err := copyFile(binaryTmp, newPath, 0755); err != nil {
		return fmt.Errorf("stage binary: %w", err)
	}
	if err := os.Rename(newPath, binaryDest); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	// Restart service; best-effort — the binary is already replaced
	_ = exec.Command("systemctl", "restart", "argus.service").Run()

	return nil
}

// archiveNameForArch returns the GoReleaser asset filename for the current architecture.
// e.g. "argus_v1.2.3_linux_arm64.tar.gz"
func archiveNameForArch(tag string) string {
	var archSuffix string
	switch runtime.GOARCH {
	case "arm64":
		archSuffix = "arm64"
	case "arm":
		archSuffix = "armv6" // conservative default for 32-bit ARM
	case "amd64":
		archSuffix = "amd64"
	default:
		archSuffix = runtime.GOARCH
	}
	return fmt.Sprintf("argus_%s_linux_%s.tar.gz", strings.TrimPrefix(tag, "v"), archSuffix)
}

func download(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func extractBinary(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Extract only the top-level "argus" binary
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == "argus" {
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			return err
		}
	}
	return fmt.Errorf("binary 'argus' not found in archive")
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
