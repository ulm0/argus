package lightshow

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulm0/argus/internal/config"
)

var validExtensions = map[string]bool{
	".fseq": true,
	".mp3":  true,
	".wav":  true,
}

type ShowGroup struct {
	BaseName  string `json:"base_name"`
	FseqFile  string `json:"fseq_file,omitempty"`
	AudioFile string `json:"audio_file,omitempty"`
	Partition string `json:"partition_key"`
}

type Service struct {
	cfg *config.Config
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

// ListShows returns light show groups from all partitions.
func (s *Service) ListShows(mountPath string) []ShowGroup {
	showDir := filepath.Join(mountPath, s.cfg.Web.LightshowFolder)
	entries, err := os.ReadDir(showDir)
	if err != nil {
		return nil
	}

	groups := make(map[string]*ShowGroup)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !validExtensions[ext] {
			continue
		}

		baseName := strings.TrimSuffix(e.Name(), ext)
		g, ok := groups[baseName]
		if !ok {
			g = &ShowGroup{BaseName: baseName}
			groups[baseName] = g
		}

		switch ext {
		case ".fseq":
			g.FseqFile = e.Name()
		case ".mp3", ".wav":
			g.AudioFile = e.Name()
		}
	}

	var result []ShowGroup
	for _, g := range groups {
		result = append(result, *g)
	}
	return result
}

// UploadFile saves an uploaded light show file to the LightShow folder.
func (s *Service) UploadFile(data []byte, filename, mountPath string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	if !validExtensions[ext] {
		return fmt.Errorf("invalid file type: %s (allowed: .fseq, .mp3, .wav)", ext)
	}

	showDir := filepath.Join(mountPath, s.cfg.Web.LightshowFolder)
	os.MkdirAll(showDir, 0755)

	destPath := filepath.Join(showDir, filename)
	return os.WriteFile(destPath, data, 0644)
}

// UploadZip extracts .fseq/.mp3/.wav files from a ZIP archive into the LightShow folder.
func (s *Service) UploadZip(zipData []byte, mountPath string) (int, error) {
	tmpFile, err := os.CreateTemp("", "lightshow-*.zip")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(zipData); err != nil {
		tmpFile.Close()
		return 0, err
	}
	tmpFile.Close()

	r, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		return 0, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	showDir := filepath.Join(mountPath, s.cfg.Web.LightshowFolder)
	os.MkdirAll(showDir, 0755)

	count := 0
	for _, f := range r.File {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if !validExtensions[ext] {
			continue
		}

		baseName := filepath.Base(f.Name) // strip paths inside ZIP
		destPath := filepath.Join(showDir, baseName)

		src, err := f.Open()
		if err != nil {
			continue
		}

		dst, err := os.Create(destPath)
		if err != nil {
			src.Close()
			continue
		}

		io.Copy(dst, src)
		dst.Close()
		src.Close()
		count++
	}
	return count, nil
}

// DeleteShow removes all files associated with a light show base name.
func (s *Service) DeleteShow(baseName, mountPath string) error {
	showDir := filepath.Join(mountPath, s.cfg.Web.LightshowFolder)

	for ext := range validExtensions {
		path := filepath.Join(showDir, baseName+ext)
		os.Remove(path) // ignore errors for non-existent files
	}
	return nil
}

// CreateDownloadZip creates a ZIP file of all files for a light show.
func (s *Service) CreateDownloadZip(baseName, mountPath string) (string, error) {
	showDir := filepath.Join(mountPath, s.cfg.Web.LightshowFolder)
	zipPath := filepath.Join(os.TempDir(), baseName+".zip")

	zipFile, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	for ext := range validExtensions {
		filePath := filepath.Join(showDir, baseName+ext)
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		fw, err := w.Create(baseName + ext)
		if err != nil {
			continue
		}
		fw.Write(data)
	}

	return zipPath, nil
}
