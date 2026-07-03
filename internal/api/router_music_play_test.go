package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ulm0/argus/internal/api/handlers"
	"github.com/ulm0/argus/internal/config"
)

// newTestRouterWithMusic builds the real router (full middleware chain) backed
// by a temp directory standing in for the mounted music partition.
func newTestRouterWithMusic(t *testing.T) (http.Handler, []byte) {
	t.Helper()
	tmp := t.TempDir()

	cfg := &config.Config{}
	cfg.MountDir = tmp
	cfg.StateFile = filepath.Join(tmp, "state")
	cfg.DiskImages.MusicEnabled = true
	cfg.Web.MaxUploadSizeMB = 100
	authOff := false
	cfg.Web.AuthEnabled = &authOff

	musicDir := filepath.Join(tmp, "part3", "Music")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 2<<20)
	for i := range data {
		data[i] = byte(i*7 + i/253)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "song.mp3"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	webFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	sentryH := handlers.NewSentryEventsHandler(cfg)
	return NewRouter(cfg, webFS, nil, sentryH, nil, nil), data
}

func playSong(t *testing.T, h http.Handler, hdrs map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/music/play/song.mp3", nil)
	req.Host = "192.168.4.1"
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// Audio playback must be served identity-encoded with Content-Length and
// working byte ranges. The gzip middleware previously compressed these
// responses (stripping Content-Length and corrupting 206 range bodies), which
// froze or glitched playback in clients that send Accept-Encoding: gzip on
// media requests.
func TestMusicPlayNotCompressed(t *testing.T) {
	h, data := newTestRouterWithMusic(t)

	t.Run("open range with gzip accepted", func(t *testing.T) {
		w := playSong(t, h, map[string]string{
			"Accept-Encoding": "gzip, deflate, br, zstd",
			"Range":           "bytes=0-",
		})
		if w.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206", w.Code)
		}
		if enc := w.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("Content-Encoding = %q, want none", enc)
		}
		if cl := w.Header().Get("Content-Length"); cl == "" {
			t.Error("Content-Length missing")
		}
		if !bytes.Equal(w.Body.Bytes(), data) {
			t.Error("body does not match file contents")
		}
	})

	t.Run("mid-file range with gzip accepted", func(t *testing.T) {
		w := playSong(t, h, map[string]string{
			"Accept-Encoding": "gzip",
			"Range":           "bytes=1000000-1499999",
		})
		if w.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206", w.Code)
		}
		if enc := w.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("Content-Encoding = %q, want none", enc)
		}
		if got, want := w.Body.Bytes(), data[1000000:1500000]; !bytes.Equal(got, want) {
			t.Errorf("range body: got %d bytes, want %d matching bytes", len(got), len(want))
		}
	})

	t.Run("chrome-style identity request still works", func(t *testing.T) {
		w := playSong(t, h, map[string]string{
			"Accept-Encoding": "identity;q=1, *;q=0",
			"Range":           "bytes=0-",
		})
		if w.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206", w.Code)
		}
		if !bytes.Equal(w.Body.Bytes(), data) {
			t.Error("body does not match file contents")
		}
	})
}
