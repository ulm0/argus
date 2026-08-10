package lightshow

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ulm0/argus/internal/config"
)

var validExtensions = map[string]bool{
	".fseq": true,
	".mp3":  true,
	".wav":  true,
}

// Bounds for ZIP extraction, to defeat zip bombs on a memory/disk-constrained Pi.
const (
	maxZipEntries    = 512
	maxZipEntryBytes = 128 << 20 // 128 MiB per extracted file
	maxZipTotalBytes = 512 << 20 // 512 MiB total per archive
)

// Header prefix inspected for fseq/WAV validation; enough for a RIFF fmt chunk
// preceded by the metadata chunks real exporters emit.
const showHeaderBytes = 4096

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
	if err := validateShowHeader(ext, data); err != nil {
		return err
	}

	showDir := filepath.Join(mountPath, s.cfg.Web.LightshowFolder)
	os.MkdirAll(showDir, 0755)

	destPath := filepath.Join(showDir, filename)
	return os.WriteFile(destPath, data, 0644)
}

// validateShowHeader rejects files the car would reject at show time, while the
// user is still standing next to the Pi. Tesla requires uncompressed fseq V2
// with a 15-100ms frame interval and under 4h, and 44.1 kHz audio (48 kHz
// drifts out of sync). MP3 is accepted as-is: probing it would mean shelling
// out to ffprobe on every upload.
func validateShowHeader(ext string, head []byte) error {
	switch ext {
	case ".fseq":
		return validateFseq(head)
	case ".wav":
		return validateShowWAV(head)
	}
	return nil
}

func validateFseq(head []byte) error {
	if len(head) < 24 || string(head[0:4]) != "PSEQ" {
		return fmt.Errorf("not an fseq file")
	}
	if head[7] != 2 {
		return fmt.Errorf("fseq v%d.%d: Tesla requires v2", head[7], head[6])
	}
	if head[20] != 0 {
		return fmt.Errorf("compressed fseq: Tesla requires uncompressed v2")
	}
	step := int(head[18])
	if step < 15 || step > 100 {
		return fmt.Errorf("frame interval %dms out of range (need 15-100ms)", step)
	}
	frames := binary.LittleEndian.Uint32(head[14:18])
	if d := time.Duration(frames) * time.Duration(step) * time.Millisecond; d > 4*time.Hour {
		return fmt.Errorf("show is %s long (max 4h)", d.Round(time.Second))
	}
	return nil
}

// validateShowWAV walks RIFF chunks looking for "fmt ". A fmt chunk beyond the
// inspected prefix is passed through rather than rejected — the point is to
// catch 48 kHz exports, not to be a general WAV validator.
func validateShowWAV(head []byte) error {
	if len(head) < 12 || string(head[0:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return fmt.Errorf("not a valid WAV file")
	}
	for off := 12; off+8 <= len(head); {
		size := binary.LittleEndian.Uint32(head[off+4 : off+8])
		if string(head[off:off+4]) == "fmt " {
			if off+16 > len(head) {
				return nil
			}
			if rate := binary.LittleEndian.Uint32(head[off+12 : off+16]); rate != 44100 {
				return fmt.Errorf("audio is %d Hz: Tesla requires 44100 Hz", rate)
			}
			return nil
		}
		if size%2 != 0 {
			size++
		}
		next := int64(off) + 8 + int64(size)
		if next > int64(len(head)) {
			return nil
		}
		off = int(next)
	}
	return nil
}

// validateShowFile checks a freshly extracted ZIP entry. Only the header
// matters, so it reads back a small prefix instead of the whole file.
func validateShowFile(path, ext string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	head := make([]byte, showHeaderBytes)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return err
	}
	return validateShowHeader(ext, head[:n])
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
	var total int64
	for _, f := range r.File {
		if count >= maxZipEntries {
			break
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if !validExtensions[ext] {
			continue
		}

		baseName := filepath.Base(f.Name) // strip paths inside ZIP
		destPath := filepath.Join(showDir, baseName)
		// Confine the write target to showDir (zip-slip guard); filepath.Base
		// already strips directory components, this makes the invariant explicit.
		if !strings.HasPrefix(destPath, showDir+string(filepath.Separator)) {
			continue
		}

		src, err := f.Open()
		if err != nil {
			continue
		}

		dst, err := os.Create(destPath)
		if err != nil {
			src.Close()
			continue
		}

		// Bound decompressed output: ParseMultipartForm only caps the *compressed*
		// upload, so a small archive could otherwise inflate to gigabytes of
		// .wav/.fseq and fill the SD card. CopyN(limit+1) lets us detect overflow.
		limit := int64(maxZipEntryBytes)
		if rem := int64(maxZipTotalBytes) - total; rem < limit {
			limit = rem
		}
		n, err := io.CopyN(dst, src, limit+1)
		dst.Close()
		src.Close()
		if err != nil && err != io.EOF {
			os.Remove(destPath)
			continue
		}
		if n > limit {
			os.Remove(destPath)
			return count, fmt.Errorf("zip entry %q exceeds size limit", baseName)
		}
		if err := validateShowFile(destPath, ext); err != nil {
			os.Remove(destPath)
			return count, fmt.Errorf("zip entry %q: %w", baseName, err)
		}
		total += n
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

	for ext := range validExtensions {
		filePath := filepath.Join(showDir, baseName+ext)
		src, err := os.Open(filePath)
		if err != nil {
			continue
		}

		fw, err := w.Create(baseName + ext)
		if err != nil {
			src.Close()
			w.Close()
			os.Remove(zipPath)
			return "", err
		}
		if _, err := io.Copy(fw, src); err != nil {
			src.Close()
			w.Close()
			os.Remove(zipPath)
			return "", err
		}
		src.Close()
	}

	if err := w.Close(); err != nil {
		os.Remove(zipPath)
		return "", err
	}

	return zipPath, nil
}
