package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/config"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "gadget", "config")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}

	mntDir := filepath.Join(dir, "mnt")
	for _, sub := range []string{"part1", "part1-ro", "part2", "part2-ro"} {
		if err := os.MkdirAll(filepath.Join(mntDir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	yaml := `
installation:
  target_user: pi
  mount_dir: ` + mntDir + `
disk_images:
  cam_name: usb_cam.img
  lightshow_name: usb_lightshow.img
network:
  samba_password: pw
web:
  max_lock_chime_size: 1048576
  max_lock_chime_duration: 10.0
  min_lock_chime_duration: 0.5
`
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func testRouter(t *testing.T, cfg *config.Config) *mux.Router {
	t.Helper()
	r := mux.NewRouter()

	modeH := NewModeHandler(cfg)

	r.HandleFunc("/api/status", modeH.Status).Methods("GET")
	r.HandleFunc("/api/operation/status", modeH.OperationStatus).Methods("GET")
	r.HandleFunc("/api/gadget/state", modeH.GadgetState).Methods("GET")

	return r
}

func TestStatusEndpoint(t *testing.T) {
	cfg := testConfig(t)
	router := testRouter(t, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal JSON error: %v\nbody: %s", err, string(body))
	}

	if _, ok := result["mode"]; !ok {
		t.Error("response missing 'mode' field")
	}
	if _, ok := result["mode_label"]; !ok {
		t.Error("response missing 'mode_label' field")
	}
	if _, ok := result["hostname"]; !ok {
		t.Error("response missing 'hostname' field")
	}
	if _, ok := result["features"]; !ok {
		t.Error("response missing 'features' field")
	}

	features, ok := result["features"].(map[string]any)
	if !ok {
		t.Fatal("'features' is not an object")
	}
	for _, key := range []string{
		"videos_available", "analytics_available",
		"chimes_available", "shows_available",
		"wraps_available", "music_available",
	} {
		if _, ok := features[key]; !ok {
			t.Errorf("features missing %q", key)
		}
	}
}

func TestOperationStatusEndpoint(t *testing.T) {
	cfg := testConfig(t)
	router := testRouter(t, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/operation/status")
	if err != nil {
		t.Fatalf("GET /api/operation/status error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal JSON error: %v\nbody: %s", err, string(body))
	}

	inProgress, ok := result["in_progress"]
	if !ok {
		t.Error("response missing 'in_progress' field")
	}
	if ip, ok := inProgress.(bool); !ok || ip {
		t.Errorf("in_progress = %v, want false (no lock files)", inProgress)
	}
}

func TestGadgetStateEndpoint(t *testing.T) {
	cfg := testConfig(t)
	router := testRouter(t, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/gadget/state")
	if err != nil {
		t.Fatalf("GET /api/gadget/state error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal JSON error: %v\nbody: %s", err, string(body))
	}

	if _, ok := result["mode"]; !ok {
		t.Error("response missing 'mode' field")
	}
}

func TestNonExistentEndpoint(t *testing.T) {
	cfg := testConfig(t)
	router := testRouter(t, cfg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/nonexistent")
	if err != nil {
		t.Fatalf("GET /api/nonexistent error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status code = %d, want 404 or 405", resp.StatusCode)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]string{"hello": "world"}
	writeJSON(w, http.StatusCreated, payload)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result["hello"] != "world" {
		t.Errorf("hello = %q, want %q", result["hello"], "world")
	}
}

func TestFullRouterIntegration(t *testing.T) {
	cfg := testConfig(t)

	webFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>test</html>")},
	}

	router := newTestFullRouter(t, cfg, webFS)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal error: %v\nbody: %s", err, string(body))
	}
	if _, ok := result["mode"]; !ok {
		t.Error("response missing 'mode' field")
	}
}

func newTestFullRouter(t *testing.T, cfg *config.Config, webFS fstest.MapFS) http.Handler {
	t.Helper()
	r := mux.NewRouter()

	modeH := NewModeHandler(cfg)

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/status", modeH.Status).Methods("GET")
	api.HandleFunc("/present", modeH.PresentUSB).Methods("POST")
	api.HandleFunc("/edit", modeH.EditUSB).Methods("POST")
	api.HandleFunc("/gadget/state", modeH.GadgetState).Methods("GET")
	api.HandleFunc("/gadget/recover", modeH.RecoverGadget).Methods("POST")
	api.HandleFunc("/operation/status", modeH.OperationStatus).Methods("GET")

	fileServer := http.FileServer(http.FS(webFS))
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fileServer.ServeHTTP(w, r)
	})

	return r
}
