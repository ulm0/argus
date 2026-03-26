package wrap

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulm0/argus/internal/config"
)

const (
	MinDimension    = 512
	MaxDimension    = 1024
	MaxFileSize     = 1 * 1024 * 1024 // 1 MiB
	MaxWrapCount    = 10
	MaxFilenameLen  = 50
	WrapsFolder     = "Wraps"
)

type WrapFile struct {
	Filename   string `json:"filename"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Size       int64  `json:"size"`
	SizeStr    string `json:"size_str"`
	Partition  string `json:"partition_key"`
}

type Service struct {
	cfg *config.Config
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

// ListWraps returns all wrap files from a mount path.
func (s *Service) ListWraps(mountPath string) []WrapFile {
	wrapsDir := filepath.Join(mountPath, WrapsFolder)
	entries, err := os.ReadDir(wrapsDir)
	if err != nil {
		return nil
	}

	var files []WrapFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			continue
		}

		filePath := filepath.Join(wrapsDir, e.Name())
		info, _ := e.Info()
		w, h, _ := GetPNGDimensions(filePath)

		wf := WrapFile{
			Filename:  e.Name(),
			Width:     w,
			Height:    h,
		}
		if info != nil {
			wf.Size = info.Size()
			wf.SizeStr = formatSize(info.Size())
		}
		files = append(files, wf)
	}
	return files
}

// GetWrapCount returns the current number of wrap files.
func (s *Service) GetWrapCount(mountPath string) int {
	wrapsDir := filepath.Join(mountPath, WrapsFolder)
	entries, err := os.ReadDir(wrapsDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			count++
		}
	}
	return count
}

// UploadWrap validates and saves a PNG wrap file.
func (s *Service) UploadWrap(data []byte, filename, mountPath string) error {
	// Validate filename
	if len(filename) > MaxFilenameLen {
		return fmt.Errorf("filename too long (max %d characters)", MaxFilenameLen)
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".png") {
		return fmt.Errorf("only PNG files allowed")
	}

	// Validate file size
	if len(data) > MaxFileSize {
		return fmt.Errorf("file too large (max 1 MiB)")
	}

	// Validate PNG header and dimensions
	w, h, err := GetPNGDimensionsFromBytes(data)
	if err != nil {
		return fmt.Errorf("invalid PNG: %w", err)
	}
	if w < MinDimension || w > MaxDimension || h < MinDimension || h > MaxDimension {
		return fmt.Errorf("invalid dimensions %dx%d (must be %d-%d)", w, h, MinDimension, MaxDimension)
	}

	// Check count limit
	count := s.GetWrapCount(mountPath)
	if count >= MaxWrapCount {
		return fmt.Errorf("maximum wrap count reached (%d)", MaxWrapCount)
	}

	wrapsDir := filepath.Join(mountPath, WrapsFolder)
	os.MkdirAll(wrapsDir, 0755)
	return os.WriteFile(filepath.Join(wrapsDir, filename), data, 0644)
}

// DeleteWrap removes a wrap PNG file.
func (s *Service) DeleteWrap(filename, mountPath string) error {
	filePath := filepath.Join(mountPath, WrapsFolder, filename)
	return os.Remove(filePath)
}

// GetPNGDimensions reads width and height from a PNG file header.
func GetPNGDimensions(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	header := make([]byte, 24)
	if _, err := f.Read(header); err != nil {
		return 0, 0, err
	}
	return parsePNGHeader(header)
}

// GetPNGDimensionsFromBytes reads width and height from PNG bytes.
func GetPNGDimensionsFromBytes(data []byte) (int, int, error) {
	if len(data) < 24 {
		return 0, 0, fmt.Errorf("data too short for PNG header")
	}
	return parsePNGHeader(data[:24])
}

func parsePNGHeader(header []byte) (int, int, error) {
	// PNG signature: 137 80 78 71 13 10 26 10
	pngSig := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	for i := 0; i < 8; i++ {
		if header[i] != pngSig[i] {
			return 0, 0, fmt.Errorf("not a PNG file")
		}
	}

	// IHDR chunk: width at offset 16, height at offset 20
	w := int(binary.BigEndian.Uint32(header[16:20]))
	h := int(binary.BigEndian.Uint32(header[20:24]))
	return w, h, nil
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %s", float64(bytes)/float64(div), []string{"KB", "MB", "GB", "TB"}[exp])
}
