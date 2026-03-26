package api

import (
	"io/fs"
	"net/http"
	"strings"

	gorillahandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/ulm0/argus/internal/api/handlers"
	"github.com/ulm0/argus/internal/api/middleware"
	"github.com/ulm0/argus/internal/config"
)

// NewRouter sets up the gorilla/mux router with all routes and middleware.
func NewRouter(cfg *config.Config, webFS fs.FS) http.Handler {
	r := mux.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logging)
	r.Use(middleware.PanicRecovery)
	r.Use(func(next http.Handler) http.Handler {
		return gorillahandlers.CompressHandler(next)
	})

	maxBody := int64(cfg.Web.MaxUploadSizeMB) * 1024 * 1024
	r.Use(func(next http.Handler) http.Handler {
		return http.MaxBytesHandler(next, maxBody)
	})

	modeH := handlers.NewModeHandler(cfg)
	videoH := handlers.NewVideoHandler(cfg)
	chimeH := handlers.NewChimeHandler(cfg)
	lightshowH := handlers.NewLightshowHandler(cfg)
	wrapH := handlers.NewWrapHandler(cfg)
	musicH := handlers.NewMusicHandler(cfg)
	analyticsH := handlers.NewAnalyticsHandler(cfg)
	cleanupH := handlers.NewCleanupHandler(cfg)
	fsckH := handlers.NewFsckHandler(cfg)
	apH := handlers.NewAPHandler(cfg)
	wifiH := handlers.NewWifiHandler(cfg)
	captiveH := handlers.NewCaptiveHandler(cfg)
	telegramH := handlers.NewTelegramHandler(cfg)
	sambaH := handlers.NewSambaHandler(cfg)

	configH := handlers.NewConfigHandler(cfg)
	logsH := handlers.NewLogsHandler(cfg)

	updateH := handlers.NewUpdateHandler(cfg)

	// Captive portal detection endpoints (must be at root)
	r.HandleFunc("/hotspot-detect.html", captiveH.Detect).Methods("GET")
	r.HandleFunc("/library/test/success.html", captiveH.Detect).Methods("GET")
	r.HandleFunc("/generate_204", captiveH.Detect).Methods("GET")
	r.HandleFunc("/gen_204", captiveH.Detect).Methods("GET")
	r.HandleFunc("/connecttest.txt", captiveH.Detect).Methods("GET")
	r.HandleFunc("/ncsi.txt", captiveH.Detect).Methods("GET")
	r.HandleFunc("/redirect", captiveH.Detect).Methods("GET")
	r.HandleFunc("/success.txt", captiveH.Detect).Methods("GET")
	r.HandleFunc("/canonical.html", captiveH.Detect).Methods("GET")

	api := r.PathPrefix("/api").Subrouter()

	// Mode control
	api.HandleFunc("/status", modeH.Status).Methods("GET")
	api.HandleFunc("/present", modeH.PresentUSB).Methods("POST")
	api.HandleFunc("/edit", modeH.EditUSB).Methods("POST")

	// AP control
	api.HandleFunc("/ap/force", apH.Force).Methods("POST")
	api.HandleFunc("/ap/configure", apH.Configure).Methods("POST")
	api.HandleFunc("/ap/status", apH.Status).Methods("GET")

	// WiFi
	api.HandleFunc("/wifi/configure", wifiH.Configure).Methods("POST")
	api.HandleFunc("/wifi/scan", wifiH.Scan).Methods("GET")
	api.HandleFunc("/wifi/dismiss-status", wifiH.DismissStatus).Methods("POST")
	api.HandleFunc("/wifi/status", wifiH.Status).Methods("GET")

	// Videos
	api.HandleFunc("/videos", videoH.List).Methods("GET")
	api.HandleFunc("/videos/{folder}/{event}", videoH.Event).Methods("GET")
	api.HandleFunc("/videos/stream/{rest:.*}", videoH.Stream).Methods("GET")
	api.HandleFunc("/videos/sei/{rest:.*}", videoH.SEI).Methods("GET")
	api.HandleFunc("/videos/download/{rest:.*}", videoH.Download).Methods("GET")
	api.HandleFunc("/videos/download-event/{folder}/{event}", videoH.DownloadEvent).Methods("GET")
	api.HandleFunc("/videos/thumbnail/{folder}/{event}", videoH.Thumbnail).Methods("GET")
	api.HandleFunc("/videos/session-thumbnail/{folder}/{session}", videoH.SessionThumbnail).Methods("GET")
	api.HandleFunc("/videos/delete/{folder}/{event}", videoH.Delete).Methods("POST")

	// Lock chimes
	api.HandleFunc("/chimes", chimeH.List).Methods("GET")
	api.HandleFunc("/chimes/play/active", chimeH.PlayActive).Methods("GET")
	api.HandleFunc("/chimes/play/{filename}", chimeH.Play).Methods("GET")
	api.HandleFunc("/chimes/download/{filename}", chimeH.Download).Methods("GET")
	api.HandleFunc("/chimes/upload", chimeH.Upload).Methods("POST")
	api.HandleFunc("/chimes/upload-bulk", chimeH.UploadBulk).Methods("POST")
	api.HandleFunc("/chimes/set/{filename}", chimeH.SetActive).Methods("POST")
	api.HandleFunc("/chimes/delete/{filename}", chimeH.Delete).Methods("POST")
	api.HandleFunc("/chimes/rename/{old}/{new}", chimeH.Rename).Methods("POST")
	api.HandleFunc("/chimes/filenames", chimeH.Filenames).Methods("GET")

	// Chime scheduler
	api.HandleFunc("/chimes/schedule/add", chimeH.AddSchedule).Methods("POST")
	api.HandleFunc("/chimes/schedule/{id}/toggle", chimeH.ToggleSchedule).Methods("POST")
	api.HandleFunc("/chimes/schedule/{id}/delete", chimeH.DeleteSchedule).Methods("POST")
	api.HandleFunc("/chimes/schedule/{id}", chimeH.GetSchedule).Methods("GET")
	api.HandleFunc("/chimes/schedule/{id}/edit", chimeH.EditSchedule).Methods("POST")

	// Chime groups
	api.HandleFunc("/chimes/groups", chimeH.ListGroups).Methods("GET")
	api.HandleFunc("/chimes/groups/create", chimeH.CreateGroup).Methods("POST")
	api.HandleFunc("/chimes/groups/{id}/update", chimeH.UpdateGroup).Methods("POST")
	api.HandleFunc("/chimes/groups/{id}/delete", chimeH.DeleteGroup).Methods("POST")
	api.HandleFunc("/chimes/groups/{id}/add-chime", chimeH.AddChimeToGroup).Methods("POST")
	api.HandleFunc("/chimes/groups/{id}/remove-chime", chimeH.RemoveChimeFromGroup).Methods("POST")
	api.HandleFunc("/chimes/groups/random-mode", chimeH.RandomMode).Methods("POST")

	// Light shows
	api.HandleFunc("/lightshows", lightshowH.List).Methods("GET")
	api.HandleFunc("/lightshows/play/{partition}/{filename}", lightshowH.Play).Methods("GET")
	api.HandleFunc("/lightshows/download/{partition}/{baseName}", lightshowH.Download).Methods("GET")
	api.HandleFunc("/lightshows/upload", lightshowH.Upload).Methods("POST")
	api.HandleFunc("/lightshows/upload-multiple", lightshowH.UploadMultiple).Methods("POST")
	api.HandleFunc("/lightshows/delete/{partition}/{baseName}", lightshowH.Delete).Methods("POST")

	// Wraps
	api.HandleFunc("/wraps", wrapH.List).Methods("GET")
	api.HandleFunc("/wraps/thumbnail/{partition}/{filename}", wrapH.Thumbnail).Methods("GET")
	api.HandleFunc("/wraps/download/{partition}/{filename}", wrapH.Download).Methods("GET")
	api.HandleFunc("/wraps/upload", wrapH.Upload).Methods("POST")
	api.HandleFunc("/wraps/upload-multiple", wrapH.UploadMultiple).Methods("POST")
	api.HandleFunc("/wraps/delete/{partition}/{filename}", wrapH.Delete).Methods("POST")

	// Music
	api.HandleFunc("/music", musicH.List).Methods("GET")
	api.HandleFunc("/music/upload", musicH.Upload).Methods("POST")
	api.HandleFunc("/music/upload-chunk", musicH.UploadChunk).Methods("POST")
	api.HandleFunc("/music/delete/{rest:.*}", musicH.Delete).Methods("POST")
	api.HandleFunc("/music/delete-dir/{rest:.*}", musicH.DeleteDir).Methods("POST")
	api.HandleFunc("/music/move", musicH.Move).Methods("POST")
	api.HandleFunc("/music/mkdir", musicH.Mkdir).Methods("POST")
	api.HandleFunc("/music/play/{rest:.*}", musicH.Play).Methods("GET")

	// Analytics
	api.HandleFunc("/analytics", analyticsH.Dashboard).Methods("GET")
	api.HandleFunc("/analytics/partition-usage", analyticsH.PartitionUsage).Methods("GET")
	api.HandleFunc("/analytics/video-stats", analyticsH.VideoStats).Methods("GET")
	api.HandleFunc("/analytics/health", analyticsH.Health).Methods("GET")

	// Cleanup
	api.HandleFunc("/cleanup/settings", cleanupH.GetSettings).Methods("GET")
	api.HandleFunc("/cleanup/settings", cleanupH.SaveSettings).Methods("POST")
	api.HandleFunc("/cleanup/preview", cleanupH.Preview).Methods("GET")
	api.HandleFunc("/cleanup/execute", cleanupH.Execute).Methods("POST")
	api.HandleFunc("/cleanup/calculate", cleanupH.Calculate).Methods("POST")

	// Fsck
	api.HandleFunc("/fsck/start", fsckH.Start).Methods("POST")
	api.HandleFunc("/fsck/status", fsckH.Status).Methods("GET")
	api.HandleFunc("/fsck/cancel", fsckH.Cancel).Methods("POST")
	api.HandleFunc("/fsck/history", fsckH.History).Methods("GET")
	api.HandleFunc("/fsck/last-check/{partition}", fsckH.LastCheck).Methods("GET")

	// Gadget state
	api.HandleFunc("/gadget/state", modeH.GadgetState).Methods("GET")
	api.HandleFunc("/gadget/recover", modeH.RecoverGadget).Methods("POST")

	// Operation status
	api.HandleFunc("/operation/status", modeH.OperationStatus).Methods("GET")

	// Telegram
	api.HandleFunc("/telegram/status", telegramH.Status).Methods("GET")
	api.HandleFunc("/telegram/configure", telegramH.Configure).Methods("POST")
	api.HandleFunc("/telegram/test", telegramH.Test).Methods("POST")

	// Samba
	api.HandleFunc("/samba/status", sambaH.Status).Methods("GET")
	api.HandleFunc("/samba/set-password", sambaH.SetPassword).Methods("POST")
	api.HandleFunc("/samba/restart", sambaH.Restart).Methods("POST")
	api.HandleFunc("/samba/regenerate", sambaH.Regenerate).Methods("POST")

	// Update
	api.HandleFunc("/update/status", updateH.Status).Methods("GET")

	// Config (editable settings)
	api.HandleFunc("/config", configH.Get).Methods("GET")
	api.HandleFunc("/config", configH.Patch).Methods("PATCH")

	// Logs (SSE journal stream)
	api.HandleFunc("/logs", logsH.Stream).Methods("GET")

	// Serve embedded Next.js static files for all non-API routes
	fileServer := http.FileServer(http.FS(webFS))
	r.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if _, err := fs.Stat(webFS, path); err != nil {
			if _, err := fs.Stat(webFS, path+".html"); err == nil {
				r.URL.Path = "/" + path + ".html"
			} else {
				// SPA fallback: serve index.html for client-side routing
				r.URL.Path = "/index.html"
			}
		}
		fileServer.ServeHTTP(w, r)
	})

	return r
}
