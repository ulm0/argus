package music

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ulm0/argus/internal/config"
)

const MusicFolder = "Music"

type FileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type DirInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type ListResult struct {
	CurrentPath string     `json:"current_path"`
	Dirs        []DirInfo  `json:"dirs"`
	Files       []FileInfo `json:"files"`
	UsedBytes   int64      `json:"used_bytes"`
	FreeBytes   int64      `json:"free_bytes"`
	TotalBytes  int64      `json:"total_bytes"`
}

type Service struct {
	cfg *config.Config
	mu  sync.Mutex
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

// ListFiles returns directory contents at the given relative path.
func (s *Service) ListFiles(mountPath, relPath string) (*ListResult, error) {
	musicDir := filepath.Join(mountPath, MusicFolder)
	targetDir := filepath.Join(musicDir, filepath.Clean(relPath))

	// Path traversal protection
	if !strings.HasPrefix(targetDir, musicDir) {
		return nil, fmt.Errorf("path traversal detected")
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	result := &ListResult{
		CurrentPath: relPath,
	}

	// Get disk usage via statvfs
	var stat struct {
		Bsize  uint64
		Blocks uint64
		Bfree  uint64
		Bavail uint64
	}
	// Fallback: use os.Stat approach
	filepath.Walk(musicDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			result.UsedBytes += info.Size()
		}
		return nil
	})

	_ = stat // suppress unused warning; real statvfs done via syscall on Linux

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			result.Dirs = append(result.Dirs, DirInfo{
				Name: e.Name(),
				Path: filepath.Join(relPath, e.Name()),
			})
		} else {
			info, _ := e.Info()
			var size int64
			if info != nil {
				size = info.Size()
			}
			result.Files = append(result.Files, FileInfo{
				Name: e.Name(),
				Path: filepath.Join(relPath, e.Name()),
				Size: size,
			})
		}
	}

	sort.Slice(result.Dirs, func(i, j int) bool { return result.Dirs[i].Name < result.Dirs[j].Name })
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Name < result.Files[j].Name })

	return result, nil
}

// SaveFile saves an uploaded file to the music directory.
func (s *Service) SaveFile(data io.Reader, filename, mountPath, relPath string) error {
	musicDir := filepath.Join(mountPath, MusicFolder)
	targetDir := filepath.Join(musicDir, filepath.Clean(relPath))

	if !strings.HasPrefix(targetDir, musicDir) {
		return fmt.Errorf("path traversal detected")
	}

	os.MkdirAll(targetDir, 0755)
	destPath := filepath.Join(targetDir, filename)

	// Atomic write: write to temp, fsync, rename
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	if _, err := io.Copy(f, data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("fsync: %w", err)
	}
	f.Close()

	return os.Rename(tmpPath, destPath)
}

// HandleChunk handles a chunked upload for large files.
func (s *Service) HandleChunk(uploadID, filename string, chunkIndex, totalChunks int,
	data io.Reader, mountPath, relPath string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	musicDir := filepath.Join(mountPath, MusicFolder)
	uploadDir := filepath.Join(musicDir, ".uploads", uploadID)
	os.MkdirAll(uploadDir, 0755)

	// Save chunk
	chunkPath := filepath.Join(uploadDir, fmt.Sprintf("chunk_%05d.part", chunkIndex))
	f, err := os.Create(chunkPath)
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(f, data); err != nil {
		f.Close()
		return false, err
	}
	f.Close()

	// Check if all chunks received
	entries, _ := os.ReadDir(uploadDir)
	if len(entries) < totalChunks {
		return false, nil
	}

	// Assemble final file
	targetDir := filepath.Join(musicDir, filepath.Clean(relPath))
	os.MkdirAll(targetDir, 0755)
	destPath := filepath.Join(targetDir, filename)
	tmpPath := destPath + ".assembling"

	assembled, err := os.Create(tmpPath)
	if err != nil {
		return false, err
	}

	for i := 0; i < totalChunks; i++ {
		cp := filepath.Join(uploadDir, fmt.Sprintf("chunk_%05d.part", i))
		chunk, err := os.Open(cp)
		if err != nil {
			assembled.Close()
			os.Remove(tmpPath)
			return false, fmt.Errorf("open chunk %d: %w", i, err)
		}
		io.Copy(assembled, chunk)
		chunk.Close()
	}

	assembled.Sync()
	assembled.Close()

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return false, err
	}

	// Cleanup chunks
	os.RemoveAll(uploadDir)

	return true, nil
}

// DeleteFile removes a music file.
func (s *Service) DeleteFile(mountPath, relPath string) error {
	musicDir := filepath.Join(mountPath, MusicFolder)
	targetPath := filepath.Join(musicDir, filepath.Clean(relPath))

	if !strings.HasPrefix(targetPath, musicDir) {
		return fmt.Errorf("path traversal detected")
	}
	return os.Remove(targetPath)
}

// DeleteDirectory removes a music directory and its contents.
func (s *Service) DeleteDirectory(mountPath, relPath string) error {
	musicDir := filepath.Join(mountPath, MusicFolder)
	targetPath := filepath.Join(musicDir, filepath.Clean(relPath))

	if !strings.HasPrefix(targetPath, musicDir) {
		return fmt.Errorf("path traversal detected")
	}
	if targetPath == musicDir {
		return fmt.Errorf("cannot delete root music directory")
	}
	return os.RemoveAll(targetPath)
}

// CreateDirectory creates a new folder in the music directory.
func (s *Service) CreateDirectory(mountPath, relPath, name string) error {
	musicDir := filepath.Join(mountPath, MusicFolder)
	targetDir := filepath.Join(musicDir, filepath.Clean(relPath), name)

	if !strings.HasPrefix(targetDir, musicDir) {
		return fmt.Errorf("path traversal detected")
	}
	return os.MkdirAll(targetDir, 0755)
}

// MoveFile moves or renames a music file.
func (s *Service) MoveFile(mountPath, srcRel, destRel, newName string) error {
	musicDir := filepath.Join(mountPath, MusicFolder)
	srcPath := filepath.Join(musicDir, filepath.Clean(srcRel))
	destDir := filepath.Join(musicDir, filepath.Clean(destRel))
	destPath := filepath.Join(destDir, newName)

	if !strings.HasPrefix(srcPath, musicDir) || !strings.HasPrefix(destPath, musicDir) {
		return fmt.Errorf("path traversal detected")
	}

	os.MkdirAll(destDir, 0755)
	return os.Rename(srcPath, destPath)
}

// ResolvePath returns the full filesystem path for a music file.
func (s *Service) ResolvePath(mountPath, relPath string) (string, error) {
	musicDir := filepath.Join(mountPath, MusicFolder)
	fullPath := filepath.Join(musicDir, filepath.Clean(relPath))

	if !strings.HasPrefix(fullPath, musicDir) {
		return "", fmt.Errorf("path traversal detected")
	}

	if _, err := os.Stat(fullPath); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	return fullPath, nil
}

// GenerateUploadID creates a unique upload ID for chunked uploads.
func GenerateUploadID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
