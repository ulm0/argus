package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/services/lightshow"
)

type LightshowHandler struct {
	cfg          *config.Config
	lightshowSvc *lightshow.Service
}

func NewLightshowHandler(cfg *config.Config) *LightshowHandler {
	return &LightshowHandler{
		cfg:          cfg,
		lightshowSvc: lightshow.NewService(cfg),
	}
}

func (h *LightshowHandler) List(w http.ResponseWriter, r *http.Request) {
	var flat []lightshow.ShowGroup

	partitions := []string{"part1", "part2"}
	if h.cfg.DiskImages.MusicEnabled {
		partitions = append(partitions, "part3")
	}

	for _, part := range partitions {
		mountPath := h.resolveMountPath(part)
		if mountPath == "" {
			continue
		}
		shows := h.lightshowSvc.ListShows(mountPath)
		for i := range shows {
			shows[i].Partition = part
		}
		flat = append(flat, shows...)
	}

	writeJSON(w, http.StatusOK, map[string]any{"shows": flat})
}

func (h *LightshowHandler) Play(w http.ResponseWriter, r *http.Request) {
	partition := mux.Vars(r)["partition"]
	filename := mux.Vars(r)["filename"]

	mountPath := h.resolveMountPath(partition)
	if mountPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid partition"})
		return
	}

	filePath := filepath.Join(mountPath, h.cfg.Web.LightshowFolder, filename)
	if _, err := os.Stat(filePath); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	http.ServeFile(w, r, filePath)
}

func (h *LightshowHandler) Download(w http.ResponseWriter, r *http.Request) {
	partition := mux.Vars(r)["partition"]
	baseName := mux.Vars(r)["baseName"]

	mountPath := h.resolveMountPath(partition)
	if mountPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid partition"})
		return
	}

	zipPath, err := h.lightshowSvc.CreateDownloadZip(baseName, mountPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer os.Remove(zipPath)

	w.Header().Set("Content-Disposition", "attachment; filename=\""+baseName+".zip\"")
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, zipPath)
}

func (h *LightshowHandler) Upload(w http.ResponseWriter, r *http.Request) {
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

	mountPath := h.editMountPath("part2")
	if mountPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "partition not available for writing"})
		return
	}

	if err := h.lightshowSvc.UploadFile(data, header.Filename, mountPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "filename": header.Filename})
}

func (h *LightshowHandler) UploadMultiple(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(int64(h.cfg.Web.MaxUploadSizeMB) << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse form"})
		return
	}

	mountPath := h.editMountPath("part2")
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

		if strings.HasSuffix(strings.ToLower(fh.Filename), ".zip") {
			count, err := h.lightshowSvc.UploadZip(data, mountPath)
			if err != nil {
				errors = append(errors, fh.Filename+": "+err.Error())
				continue
			}
			uploaded += count
		} else {
			if err := h.lightshowSvc.UploadFile(data, fh.Filename, mountPath); err != nil {
				errors = append(errors, fh.Filename+": "+err.Error())
				continue
			}
			uploaded++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"uploaded": uploaded,
		"errors":   errors,
	})
}

func (h *LightshowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	partition := mux.Vars(r)["partition"]
	baseName := mux.Vars(r)["baseName"]

	mountPath := h.editMountPath(partition)
	if mountPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid partition"})
		return
	}

	if err := h.lightshowSvc.DeleteShow(baseName, mountPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *LightshowHandler) resolveMountPath(partition string) string {
	for _, ro := range []bool{true, false} {
		path := h.cfg.MountPath(partition, ro)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func (h *LightshowHandler) editMountPath(partition string) string {
	path := h.cfg.MountPath(partition, false)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return ""
}
