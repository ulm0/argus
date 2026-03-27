"use client";

import { useEffect, useState, useCallback } from "react";
import type React from "react";
import * as api from "@/lib/api";
import type { ConfigResponse } from "@/lib/types";

const inputCls =
  "mt-1 block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-3 py-2.5 text-sm text-[var(--color-text-primary)] shadow-sm transition-all placeholder:text-[var(--color-text-muted)] focus:border-[var(--color-accent)] focus:outline-none focus:ring-2 focus:ring-[var(--color-accent)]/20 disabled:opacity-50";

const cardCls = "rounded bg-[var(--color-bg-card)] p-6 shadow-sm";

const btnPrimaryCls =
  "rounded bg-[var(--color-accent)] px-5 py-2.5 text-sm font-semibold text-white transition-all hover:bg-[var(--color-accent-hover)] active:scale-[0.98] disabled:opacity-50";

const sectionTitle = "mb-4 text-base font-semibold text-[var(--color-text-primary)]";
const fieldLabel = "block text-xs font-medium text-[var(--color-text-muted)] uppercase tracking-wide";
const roValue = "mt-1 block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-secondary)] px-3 py-2.5 text-sm text-[var(--color-text-secondary)] select-all";
const badgeCls = (active: boolean) =>
  `inline-flex items-center rounded px-2 py-0.5 text-xs font-semibold ${
    active
      ? "bg-[var(--color-accent)]/15 text-[var(--color-accent)]"
      : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"
  }`;

function SectionHeader({ title, description }: { title: React.ReactNode; description?: string }) {
  return (
    <div className="mb-6 border-b border-[var(--color-border)] pb-3">
      <h2 className="text-lg font-bold text-[var(--color-text-primary)]">{title}</h2>
      {description && (
        <p className="mt-1 text-sm text-[var(--color-text-muted)]">{description}</p>
      )}
    </div>
  );
}

function ReadOnlyBadge() {
  return (
    <span className="ml-2 rounded bg-[var(--color-bg-tertiary)] px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-muted)]">
      read-only
    </span>
  );
}

export default function SettingsPage() {
  const [cfg, setCfg] = useState<ConfigResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<string | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  // Editable states — Web
  const [webMaxChimeSize, setWebMaxChimeSize] = useState(0);
  const [webMaxChimeDur, setWebMaxChimeDur] = useState(0);
  const [webMinChimeDur, setWebMinChimeDur] = useState(0);
  const [webSpeedMin, setWebSpeedMin] = useState(0);
  const [webSpeedMax, setWebSpeedMax] = useState(0);
  const [webSpeedStep, setWebSpeedStep] = useState(0);
  const [webMaxUpload, setWebMaxUpload] = useState(0);
  const [webMaxChunk, setWebMaxChunk] = useState(0);

  // Editable states — Log Level
  const [logLevel, setLogLevel] = useState("debug");

  // Editable states — Update
  const [updateAuto, setUpdateAuto] = useState(false);
  const [updateCheckOnStartup, setUpdateCheckOnStartup] = useState(true);

  // Editable states — Startup & reliability
  const [bootPresentOnStart, setBootPresentOnStart] = useState(false);
  const [bootBlockUntilReady, setBootBlockUntilReady] = useState(false);
  const [bootCleanupOnStart, setBootCleanupOnStart] = useState(false);
  const [bootRandomChimeOnStart, setBootRandomChimeOnStart] = useState(false);
  const [bootFsckEnabled, setBootFsckEnabled] = useState(true);
  const [watchdogEnabled, setWatchdogEnabled] = useState(false);
  const [watchdogTimeoutSec, setWatchdogTimeoutSec] = useState(60);
  const [reapplySysctlOnStart, setReapplySysctlOnStart] = useState(false);

  // Editable states — AP advanced
  const [apCheckInterval, setApCheckInterval] = useState(0);
  const [apDisconnectGrace, setApDisconnectGrace] = useState(0);
  const [apMinRSSI, setApMinRSSI] = useState(0);
  const [apStableSeconds, setApStableSeconds] = useState(0);
  const [apPingTarget, setApPingTarget] = useState("");
  const [apRetrySeconds, setApRetrySeconds] = useState(0);

  const showToast = useCallback((msg: string, ok = true) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadConfig = useCallback(async () => {
    try {
      const data = await api.getConfig();
      setCfg(data);
      setLogLevel(data.log_level || "debug");
      setWebMaxChimeSize(data.web.max_lock_chime_size);
      setWebMaxChimeDur(data.web.max_lock_chime_duration);
      setWebMinChimeDur(data.web.min_lock_chime_duration);
      setWebSpeedMin(data.web.speed_range_min);
      setWebSpeedMax(data.web.speed_range_max);
      setWebSpeedStep(data.web.speed_step);
      setWebMaxUpload(data.web.max_upload_size_mb);
      setWebMaxChunk(data.web.max_upload_chunk_mb);
      setUpdateAuto(data.update.auto_update);
      setUpdateCheckOnStartup(data.update.check_on_startup);
      setBootPresentOnStart(data.startup.boot_present_on_start);
      setBootBlockUntilReady(data.startup.boot_block_until_ready);
      setBootCleanupOnStart(data.startup.boot_cleanup_on_start);
      setBootRandomChimeOnStart(data.startup.boot_random_chime_on_start);
      setBootFsckEnabled(data.startup.boot_fsck_enabled);
      setWatchdogEnabled(data.startup.watchdog_enabled);
      setWatchdogTimeoutSec(data.startup.watchdog_timeout_sec);
      setReapplySysctlOnStart(data.startup.reapply_sysctl_on_start);
      setApCheckInterval(data.offline_ap.check_interval);
      setApDisconnectGrace(data.offline_ap.disconnect_grace);
      setApMinRSSI(data.offline_ap.min_rssi);
      setApStableSeconds(data.offline_ap.stable_seconds);
      setApPingTarget(data.offline_ap.ping_target);
      setApRetrySeconds(data.offline_ap.retry_seconds);
    } catch {
      showToast("Failed to load configuration", false);
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    loadConfig();
  }, [loadConfig]);

  const saveSection = useCallback(
    async (section: string, patch: Parameters<typeof api.patchConfig>[0]) => {
      setSaving(section);
      try {
        await api.patchConfig(patch);
        showToast("Settings saved");
        await loadConfig();
      } catch (e) {
        showToast(e instanceof Error ? e.message : "Failed to save", false);
      } finally {
        setSaving(null);
      }
    },
    [showToast, loadConfig],
  );

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center p-8">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-[var(--color-accent)] border-t-transparent" />
      </div>
    );
  }

  if (!cfg) {
    return (
      <div className="flex min-h-full items-center justify-center p-8">
        <p className="text-sm text-[var(--color-text-muted)]">Failed to load configuration.</p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl space-y-8 p-6">
      {/* Toast */}
      {toast && (
        <div
          className={`fixed bottom-6 right-6 z-50 rounded px-4 py-3 text-sm font-medium shadow-lg transition-all ${
            toast.ok
              ? "bg-[var(--color-accent)] text-white"
              : "bg-red-500 text-white"
          }`}
        >
          {toast.msg}
        </div>
      )}

      <div>
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Settings</h1>
        <p className="mt-1 text-sm text-[var(--color-text-muted)]">
          View and edit application configuration. Storage settings are set once at setup and
          cannot be changed here.
        </p>
      </div>

      {/* ── Log Level ──────────────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Logging"
          description="Set the application log verbosity. Changes take effect immediately."
        />
        <div className="max-w-xs">
          <label className={fieldLabel} htmlFor="log-level">
            Log level
          </label>
          <select
            id="log-level"
            value={logLevel}
            onChange={(e) => setLogLevel(e.target.value)}
            className={inputCls}
          >
            <option value="trace">Trace</option>
            <option value="debug">Debug</option>
            <option value="info">Info</option>
            <option value="warning">Warning</option>
            <option value="error">Error</option>
            <option value="fatal">Fatal</option>
          </select>
        </div>
        <div className="mt-5 flex justify-end">
          <button
            className={btnPrimaryCls}
            disabled={saving === "log_level"}
            onClick={() =>
              saveSection("log_level", { log_level: logLevel })
            }
          >
            {saving === "log_level" ? "Saving…" : "Save"}
          </button>
        </div>
      </div>

      {/* ── Storage (read-only) ─────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title={<>Storage<ReadOnlyBadge /></>}
          description="Partition layout configured at setup time. To change these, re-run argus setup."
        />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <span className={fieldLabel}>Install directory</span>
            <div className={roValue}>{cfg.storage.install_dir}</div>
          </div>
          <div>
            <span className={fieldLabel}>Mount directory</span>
            <div className={roValue}>{cfg.storage.mount_dir}</div>
          </div>
          <div>
            <span className={fieldLabel}>System user</span>
            <div className={roValue}>{cfg.storage.target_user}</div>
          </div>
          <div>
            <span className={fieldLabel}>Music filesystem</span>
            <div className={roValue}>{cfg.storage.music_fs}</div>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          {[
            { label: "TeslaCam", always: true },
            { label: "LightShow", active: cfg.storage.part2_enabled },
            { label: "Chimes", active: cfg.storage.chimes_enabled && cfg.storage.part2_enabled },
            { label: "Wraps", active: cfg.storage.wraps_enabled && cfg.storage.part2_enabled },
            { label: "Music", active: cfg.storage.music_enabled },
          ].map(({ label, active, always }) => (
            <span key={label} className={badgeCls(always ?? active ?? false)}>
              {label}
            </span>
          ))}
        </div>
      </div>

      {/* ── Update ─────────────────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Updates"
          description="Control how Argus checks for and installs new releases."
        />
        <div className="space-y-4">
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={updateAuto}
              onChange={(e) => setUpdateAuto(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">
              Auto-update on new release
            </span>
          </label>
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={updateCheckOnStartup}
              onChange={(e) => setUpdateCheckOnStartup(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">
              Check for updates on startup
            </span>
          </label>
        </div>
        <div className="mt-5 flex justify-end">
          <button
            className={btnPrimaryCls}
            disabled={saving === "update"}
            onClick={() =>
              saveSection("update", {
                update: { auto_update: updateAuto, check_on_startup: updateCheckOnStartup },
              })
            }
          >
            {saving === "update" ? "Saving…" : "Save"}
          </button>
        </div>
      </div>

      {/* ── Startup & Reliability ─────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Startup & Reliability"
          description="Controls for unattended boot behavior. Most options apply on next service start/boot."
        />
        <div className="space-y-4">
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={bootPresentOnStart}
              onChange={(e) => setBootPresentOnStart(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">Present USB automatically on startup</span>
          </label>
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={bootBlockUntilReady}
              onChange={(e) => setBootBlockUntilReady(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">Block startup until boot pipeline completes</span>
          </label>
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={bootCleanupOnStart}
              onChange={(e) => setBootCleanupOnStart(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">Run boot cleanup on startup</span>
          </label>
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={bootRandomChimeOnStart}
              onChange={(e) => setBootRandomChimeOnStart(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">Pick a random chime on startup</span>
          </label>
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={bootFsckEnabled}
              onChange={(e) => setBootFsckEnabled(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">Run boot fsck checks</span>
          </label>
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={watchdogEnabled}
              onChange={(e) => setWatchdogEnabled(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">Enable hardware watchdog</span>
          </label>
          <label className={fieldLabel} htmlFor="watchdog-timeout-sec">
            Watchdog timeout (seconds)
          </label>
          <input
            id="watchdog-timeout-sec"
            type="number"
            min={1}
            className={inputCls}
            value={watchdogTimeoutSec}
            onChange={(e) => setWatchdogTimeoutSec(Number(e.target.value))}
          />
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={reapplySysctlOnStart}
              onChange={(e) => setReapplySysctlOnStart(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">Reapply sysctl profile on startup</span>
          </label>
        </div>
        <div className="mt-5 flex justify-end">
          <button
            className={btnPrimaryCls}
            disabled={saving === "startup"}
            onClick={() =>
              saveSection("startup", {
                startup: {
                  boot_present_on_start: bootPresentOnStart,
                  boot_block_until_ready: bootBlockUntilReady,
                  boot_cleanup_on_start: bootCleanupOnStart,
                  boot_random_chime_on_start: bootRandomChimeOnStart,
                  boot_fsck_enabled: bootFsckEnabled,
                  watchdog_enabled: watchdogEnabled,
                  watchdog_timeout_sec: watchdogTimeoutSec,
                  reapply_sysctl_on_start: reapplySysctlOnStart,
                },
              })
            }
          >
            {saving === "startup" ? "Saving…" : "Save"}
          </button>
        </div>
      </div>

      {/* ── AP Advanced ────────────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Access Point — Advanced"
          description="Fine-tune AP health-check timing. For SSID/passphrase, use the Dashboard."
        />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className={fieldLabel} htmlFor="ap-check-interval">
              Health-check interval (s)
            </label>
            <input
              id="ap-check-interval"
              type="number"
              className={inputCls}
              value={apCheckInterval}
              onChange={(e) => setApCheckInterval(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="ap-disconnect-grace">
              Disconnect grace (s)
            </label>
            <input
              id="ap-disconnect-grace"
              type="number"
              className={inputCls}
              value={apDisconnectGrace}
              onChange={(e) => setApDisconnectGrace(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="ap-min-rssi">
              Min RSSI (dBm)
            </label>
            <input
              id="ap-min-rssi"
              type="number"
              className={inputCls}
              value={apMinRSSI}
              onChange={(e) => setApMinRSSI(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="ap-stable-seconds">
              Stable seconds
            </label>
            <input
              id="ap-stable-seconds"
              type="number"
              className={inputCls}
              value={apStableSeconds}
              onChange={(e) => setApStableSeconds(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="ap-ping-target">
              Ping target
            </label>
            <input
              id="ap-ping-target"
              type="text"
              className={inputCls}
              value={apPingTarget}
              onChange={(e) => setApPingTarget(e.target.value)}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="ap-retry-seconds">
              STA retry interval (s)
            </label>
            <input
              id="ap-retry-seconds"
              type="number"
              className={inputCls}
              value={apRetrySeconds}
              onChange={(e) => setApRetrySeconds(Number(e.target.value))}
            />
          </div>
        </div>
        <div className="mt-5 flex justify-end">
          <button
            className={btnPrimaryCls}
            disabled={saving === "ap"}
            onClick={() =>
              saveSection("ap", {
                offline_ap: {
                  check_interval: apCheckInterval,
                  disconnect_grace: apDisconnectGrace,
                  min_rssi: apMinRSSI,
                  stable_seconds: apStableSeconds,
                  ping_target: apPingTarget,
                  retry_seconds: apRetrySeconds,
                },
              })
            }
          >
            {saving === "ap" ? "Saving…" : "Save"}
          </button>
        </div>
      </div>

      {/* ── Web / Chimes Limits ─────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Chimes &amp; Upload Limits"
          description="Constraints applied when uploading chime audio files and media."
        />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className={fieldLabel} htmlFor="chime-max-size">
              Max chime file size (bytes)
            </label>
            <input
              id="chime-max-size"
              type="number"
              className={inputCls}
              value={webMaxChimeSize}
              onChange={(e) => setWebMaxChimeSize(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="chime-max-dur">
              Max chime duration (s)
            </label>
            <input
              id="chime-max-dur"
              type="number"
              step="0.1"
              className={inputCls}
              value={webMaxChimeDur}
              onChange={(e) => setWebMaxChimeDur(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="chime-min-dur">
              Min chime duration (s)
            </label>
            <input
              id="chime-min-dur"
              type="number"
              step="0.1"
              className={inputCls}
              value={webMinChimeDur}
              onChange={(e) => setWebMinChimeDur(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="speed-min">
              Playback speed min
            </label>
            <input
              id="speed-min"
              type="number"
              step="0.05"
              className={inputCls}
              value={webSpeedMin}
              onChange={(e) => setWebSpeedMin(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="speed-max">
              Playback speed max
            </label>
            <input
              id="speed-max"
              type="number"
              step="0.05"
              className={inputCls}
              value={webSpeedMax}
              onChange={(e) => setWebSpeedMax(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="speed-step">
              Playback speed step
            </label>
            <input
              id="speed-step"
              type="number"
              step="0.01"
              className={inputCls}
              value={webSpeedStep}
              onChange={(e) => setWebSpeedStep(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="max-upload">
              Max upload size (MiB)
            </label>
            <input
              id="max-upload"
              type="number"
              className={inputCls}
              value={webMaxUpload}
              onChange={(e) => setWebMaxUpload(Number(e.target.value))}
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="max-chunk">
              Upload chunk size (MiB)
            </label>
            <input
              id="max-chunk"
              type="number"
              className={inputCls}
              value={webMaxChunk}
              onChange={(e) => setWebMaxChunk(Number(e.target.value))}
            />
          </div>
        </div>
        <div className="mt-5 flex justify-end">
          <button
            className={btnPrimaryCls}
            disabled={saving === "web"}
            onClick={() =>
              saveSection("web", {
                web: {
                  max_lock_chime_size: webMaxChimeSize,
                  max_lock_chime_duration: webMaxChimeDur,
                  min_lock_chime_duration: webMinChimeDur,
                  speed_range_min: webSpeedMin,
                  speed_range_max: webSpeedMax,
                  speed_step: webSpeedStep,
                  max_upload_size_mb: webMaxUpload,
                  max_upload_chunk_mb: webMaxChunk,
                },
              })
            }
          >
            {saving === "web" ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
