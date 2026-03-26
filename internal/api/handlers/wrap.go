package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/wrap"
)

type WrapHandler struct {
	cfg     *config.Config
	wrapSvc *wrap.Service
}

func NewWrapHandler(cfg *config.Config) *WrapHandler {
	return &WrapHandler{
		cfg:     cfg,
		wrapSvc: wrap.NewService(cfg),
	}
}

func (h *WrapHandler) List(w http.ResponseWriter, r *http.Request) {
	var allWraps []wrap.WrapFile

	partitions := []string{"part1", "part2"}
	if h.cfg.DiskImages.MusicEnabled {
		partitions = append(partitions, "part3")
	}

	for _, part := range partitions {
		mountPath := h.resolveWrapMount(part)
		if mountPath == "" {
			continue
		}
		wraps := h.wrapSvc.ListWraps(mountPath)
		for i := range wraps {
			wraps[i].Partition = part
		}
		allWraps = append(allWraps, wraps...)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"wraps":     allWraps,
		"max_count": wrap.MaxWrapCount,
		"max_size":  wrap.MaxFileSize,
	})
}

func (h *WrapHandler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	partition := mux.Vars(r)["partition"]
	filename := mux.Vars(r)["filename"]

	mountPath := h.resolveWrapMount(partition)
	if mountPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid partition"})
		return
	}

	base := filepath.Join(mountPath, wrap.WrapsFolder)
	filePath := filepath.Join(base, filepath.Clean(filename))
	if !strings.HasPrefix(filePath, base+string(filepath.Separator)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
		return
	}
	if _, err := os.Stat(filePath); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	http.ServeFile(w, r, filePath)
}

func (h *WrapHandler) Download(w http.ResponseWriter, r *http.Request) {
	partition := mux.Vars(r)["partition"]
	filename := mux.Vars(r)["filename"]

	mountPath := h.resolveWrapMount(partition)
	if mountPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid partition"})
		return
	}

	base := filepath.Join(mountPath, wrap.WrapsFolder)
	filePath := filepath.Join(base, filepath.Clean(filename))
	if !strings.HasPrefix(filePath, base+string(filepath.Separator)) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid filename"})
		return
	}
	if _, err := os.Stat(filePath); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, filePath)
}

func (h *WrapHandler) Upload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file field"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read file"})
		return
	}

	mountPath := h.editWrapMount("part2")
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available for writing"})
		return
	}

	if err := h.wrapSvc.UploadWrap(data, header.Filename, mountPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "filename": header.Filename})
}

func (h *WrapHandler) UploadMultiple(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(int64(h.cfg.Web.MaxUploadSizeMB) << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse form"})
		return
	}

	mountPath := h.editWrapMount("part2")
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available for writing"})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no files provided"})
		return
	}

	uploaded := 0
	var errors []string

	for _, fh := range files {
		if !strings.HasSuffix(strings.ToLower(fh.Filename), ".png") {
			errors = append(errors, fh.Filename+": only PNG files allowed")
			continue
		}

		file, err := fh.Open()
		if err != nil {
			errors = append(errors, fh.Filename+": "+err.Error())
			continue
		}

		data, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			errors = append(errors, fh.Filename+": read error")
			continue
		}

		if err := h.wrapSvc.UploadWrap(data, fh.Filename, mountPath); err != nil {
			errors = append(errors, fh.Filename+": "+err.Error())
			continue
		}
		uploaded++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"uploaded": uploaded,
		"errors":   errors,
	})
}

func (h *WrapHandler) Delete(w http.ResponseWriter, r *http.Request) {
	partition := mux.Vars(r)["partition"]
	filename := mux.Vars(r)["filename"]

	mountPath := h.editWrapMount(partition)
	if mountPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid partition"})
		return
	}

	if err := h.wrapSvc.DeleteWrap(filename, mountPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *WrapHandler) resolveWrapMount(partition string) string {
	for _, ro := range []bool{true, false} {
		path := h.cfg.MountPath(partition, ro)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func (h *WrapHandler) editWrapMount(partition string) string {
	path := h.cfg.MountPath(partition, false)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return ""
}
