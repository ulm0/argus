package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/ulm0/argus/internal/api"
	"github.com/ulm0/argus/internal/api/handlers"
	"github.com/ulm0/argus/internal/config"
	"github.com/ulm0/argus/internal/logger"
	"github.com/ulm0/argus/internal/services/telegram"
	"github.com/ulm0/argus/internal/updater"
)

func NewRunCmd(webContent *embed.FS) *cobra.Command {
	var cfgPath string

	c := &cobra.Command{
		Use:   "run",
		Short: "Start the Argus web server",
		Long:  "Start the Argus HTTP server serving the web UI and REST API.",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := resolveCfgPath(cfgPath, args)
			if err := config.InitGlobal(resolved); err != nil {
				return fmt.Errorf("failed to load config from %s: %w", resolved, err)
			}
			cfg := config.Get()
			logger.L.WithField("gadget_dir", cfg.GadgetDir).WithField("port", cfg.Network.WebPort).Info("Argus starting")

			handlers.SetVersionProvider(func() string { return Version })
			go checkForUpdate(cfg)

			webFS, err := fs.Sub(*webContent, "web/out")
			if err != nil {
				return fmt.Errorf("failed to access embedded web content: %w", err)
			}

			router := api.NewRouter(cfg, webFS)

			addr := fmt.Sprintf(":%d", cfg.Network.WebPort)
			server := &http.Server{
				Addr:    addr,
				Handler: router,
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				sig := <-sigCh
				logger.L.WithField("signal", sig).Info("received signal, shutting down")
				server.Close()
			}()

			logger.L.WithField("addr", addr).Info("listening")
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("server error: %w", err)
			}
			logger.L.Info("Argus stopped")
			return nil
		},
	}

	c.Flags().StringVarP(&cfgPath, "config", "c", "", "path to config.yaml (also reads ARGUS_CONFIG env var)")
	return c
}

// resolveCfgPath returns the config path from flag, positional arg, env var, or auto-detection.
func resolveCfgPath(flagVal string, args []string) string {
	if flagVal != "" {
		return flagVal
	}
	if len(args) > 0 {
		return args[0]
	}
	if p := os.Getenv("ARGUS_CONFIG"); p != "" {
		return p
	}
	// Check ~/.argus/config.yaml first (standard hidden data dir)
	if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, ".argus", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for _, rel := range []string{"config.yaml", "../../config.yaml"} {
			candidate := filepath.Join(dir, rel)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	logger.L.Fatal("cannot find config.yaml — pass --config or set ARGUS_CONFIG")
	return ""
}

// checkForUpdate runs at startup (non-blocking goroutine): checks GitHub for a newer release,
// logs and optionally notifies via Telegram, then auto-installs if opted in.
func checkForUpdate(cfg *config.Config) {
	if !cfg.Update.CheckOnStartup {
		return
	}
	if !updater.IsOnline() {
		return
	}
	release, err := updater.CheckLatest(Version)
	if err != nil {
		logger.L.WithError(err).Warn("update check failed")
		return
	}
	if release == nil {
		return
	}

	logger.L.WithField("latest", release.Version).WithField("current", Version).Info("new Argus version available — run `sudo argus upgrade` to update")
	updater.SetPendingRelease(release)

	if cfg.Telegram.Enabled {
		msg := fmt.Sprintf("New Argus version available: %s (current: %s). Run `sudo argus upgrade` to update.", release.Version, Version)
		tg := telegram.NewService(cfg)
		if err := tg.NotifyText(msg); err != nil {
			logger.L.WithError(err).Warn("Telegram update notification failed")
		}
	}

	if cfg.Update.AutoUpdate {
		logger.L.Info("auto-update enabled — installing")
		if err := updater.Install(release); err != nil {
			logger.L.WithError(err).Error("auto-update failed")
		}
	}
}
