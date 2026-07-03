"use client";

import { useEffect, useState, useCallback } from "react";
import type React from "react";
import * as api from "@/lib/api";
import type { ConfigResponse, APStatus, WifiStatus, WifiNetwork, SavedWifiNetwork, BluetoothStatus, BluetoothDevice, TelegramStatus, SambaStatus } from "@/lib/types";
import { useSpeedUnit } from "@/lib/useSpeedUnit";
import type { SpeedUnit } from "@/lib/useSpeedUnit";
import { useMapTheme } from "@/lib/useMapTheme";
import type { MapTheme } from "@/lib/useMapTheme";

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

  // Editable states — Webhook
  const [webhookEnabled, setWebhookEnabled] = useState(false);
  const [webhookURL, setWebhookURL] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");

  // Network — AP basic
  const [apStatus, setApStatus] = useState<APStatus | null>(null);
  const [apSSID, setApSSID] = useState("");
  const [apPass, setApPass] = useState("");
  const [showApPass, setShowApPass] = useState(false);

  // Network — WiFi client
  const [wifiStatus, setWifiStatus] = useState<WifiStatus | null>(null);
  const [wifiNetworks, setWifiNetworks] = useState<WifiNetwork[]>([]);
  const [wifiSSID, setWifiSSID] = useState("");
  const [wifiPass, setWifiPass] = useState("");
  const [scanning, setScanning] = useState(false);
  const [savedWifi, setSavedWifi] = useState<SavedWifiNetwork[]>([]);
  const [btStatus, setBtStatus] = useState<BluetoothStatus | null>(null);
  const [btDevices, setBtDevices] = useState<BluetoothDevice[]>([]);
  const [btScanning, setBtScanning] = useState(false);

  // Notifications — Telegram
  const [telegramStatus, setTelegramStatus] = useState<TelegramStatus | null>(null);
  const [tgBotToken, setTgBotToken] = useState("");
  const [tgChatID, setTgChatID] = useState("");
  const [tgOffline, setTgOffline] = useState("queue");
  const [tgQuality, setTgQuality] = useState("hd");

  // File sharing — Samba
  const [sambaStatus, setSambaStatus] = useState<SambaStatus | null>(null);
  const [sambaPass, setSambaPass] = useState("");
  const [sambaPassConfirm, setSambaPassConfirm] = useState("");

  // Display preferences (localStorage only)
  const [speedUnit, setSpeedUnit] = useSpeedUnit();
  const [mapTheme, setMapTheme] = useMapTheme();

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

  const loadWebhook = useCallback(async () => {
    try {
      const data = await api.getWebhookStatus();
      setWebhookEnabled(data.enabled);
      setWebhookURL(data.url);
    } catch {}
  }, []);

  const loadNetwork = useCallback(async () => {
    const [ap, wifi, saved, bt, btDev, tg, smb] = await Promise.allSettled([
      api.getAPStatus(),
      api.getWifiStatus(),
      api.getSavedWifi(),
      api.getBluetoothStatus(),
      api.getBluetoothDevices(),
      api.getTelegramStatus(),
      api.getSambaStatus(),
    ]);
    if (ap.status === "fulfilled") {
      setApStatus(ap.value);
      setApSSID(ap.value.ssid || "");
    }
    if (wifi.status === "fulfilled") {
      setWifiStatus(wifi.value);
      if (wifi.value.connection?.ssid) setWifiSSID(wifi.value.connection.ssid);
    }
    if (saved.status === "fulfilled") setSavedWifi(saved.value.networks || []);
    if (bt.status === "fulfilled") setBtStatus(bt.value);
    if (btDev.status === "fulfilled") setBtDevices(btDev.value.devices || []);
    if (tg.status === "fulfilled") setTelegramStatus(tg.value);
    if (smb.status === "fulfilled") setSambaStatus(smb.value);
  }, []);

  const refreshSavedWifi = useCallback(async () => {
    try {
      const res = await api.getSavedWifi();
      setSavedWifi(res.networks || []);
    } catch {}
  }, []);

  const refreshBluetooth = useCallback(async () => {
    const [st, dev] = await Promise.allSettled([
      api.getBluetoothStatus(),
      api.getBluetoothDevices(),
    ]);
    if (st.status === "fulfilled") setBtStatus(st.value);
    if (dev.status === "fulfilled") setBtDevices(dev.value.devices || []);
  }, []);

  const connectBluetooth = useCallback(async (mac: string, name: string) => {
    try {
      await api.bluetoothConnect(mac);
      showToast(`Connecting to ${name}…`);
      await refreshBluetooth();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Bluetooth connect failed", false);
    }
  }, [showToast, refreshBluetooth]);

  const disconnectBluetooth = useCallback(async (mac: string, name: string) => {
    try {
      await api.bluetoothDisconnect(mac);
      showToast(`Disconnected ${name}`);
      await refreshBluetooth();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Bluetooth disconnect failed", false);
    }
  }, [showToast, refreshBluetooth]);

  const toggleBluetoothPower = useCallback(async (enabled: boolean) => {
    try {
      await api.setBluetoothPower(enabled);
      showToast(enabled ? "Bluetooth on" : "Bluetooth off");
      await refreshBluetooth();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  }, [showToast, refreshBluetooth]);

  const toggleBluetoothDiscoverable = useCallback(async (enabled: boolean) => {
    try {
      await api.setBluetoothDiscoverable(enabled);
      showToast(enabled ? "Pi is now discoverable — pair from your phone" : "Discoverable off");
      await refreshBluetooth();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  }, [showToast, refreshBluetooth]);

  const scanBluetooth = useCallback(async () => {
    setBtScanning(true);
    try {
      const res = await api.scanBluetooth();
      setBtDevices(res.devices || []);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Bluetooth scan failed", false);
    } finally {
      setBtScanning(false);
    }
  }, [showToast]);

  const pairBluetooth = useCallback(async (mac: string, name: string) => {
    try {
      await api.bluetoothPair(mac);
      showToast(`Paired with ${name}`);
      await refreshBluetooth();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Bluetooth pair failed", false);
    }
  }, [showToast, refreshBluetooth]);

  const removeBluetooth = useCallback(async (mac: string, name: string) => {
    if (!confirm(`Forget Bluetooth device "${name}"?`)) return;
    try {
      await api.bluetoothRemove(mac);
      showToast(`Removed ${name}`);
      await refreshBluetooth();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  }, [showToast, refreshBluetooth]);

  const forgetWifi = useCallback(async (uuid: string, name: string) => {
    if (!confirm(`Forget WiFi network "${name}"? The device will no longer auto-join it.`)) return;
    try {
      await api.forgetWifi(uuid);
      showToast(`Forgot ${name}`);
      refreshSavedWifi();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed to forget network", false);
    }
  }, [showToast, refreshSavedWifi]);

  const toggleWifiAutoConnect = useCallback(async (n: SavedWifiNetwork) => {
    try {
      await api.setWifiAutoConnect(n.uuid, !n.autoconnect);
      refreshSavedWifi();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed to update network", false);
    }
  }, [showToast, refreshSavedWifi]);

  const copyText = useCallback(async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      showToast(`${label} copied`);
    } catch {
      showToast("Copy failed — clipboard unavailable", false);
    }
  }, [showToast]);

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

  const saveWebhook = useCallback(async () => {
    setSaving("webhook");
    try {
      await api.configureWebhook({ enabled: webhookEnabled, url: webhookURL, secret: webhookSecret });
      setWebhookSecret("");
      showToast("Webhook settings saved");
      await loadWebhook();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed to save webhook", false);
    } finally {
      setSaving(null);
    }
  }, [webhookEnabled, webhookURL, webhookSecret, showToast, loadWebhook]);

  const saveAP = useCallback(async () => {
    setSaving("ap-basic");
    try {
      await api.configureAP({ ssid: apSSID, passphrase: apPass });
      setApPass("");
      showToast("Access point configuration saved");
      const data = await api.getAPStatus();
      setApStatus(data);
      setApSSID(data.ssid || "");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    } finally {
      setSaving(null);
    }
  }, [apSSID, apPass, showToast]);

  const forceAP = useCallback(async (mode: "auto" | "on" | "off") => {
    try {
      await api.forceAP(mode);
      showToast(`AP mode: ${mode}`);
      setApStatus(await api.getAPStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  }, [showToast]);

  const toggleShareInternet = useCallback(async (enabled: boolean) => {
    try {
      await api.setAPShareInternet(enabled);
      showToast(enabled ? "Internet sharing enabled" : "Internet sharing disabled");
      setApStatus(await api.getAPStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  }, [showToast]);

  const connectWifi = useCallback(async () => {
    setSaving("wifi");
    try {
      await api.configureWifi(wifiSSID, wifiPass);
      setWifiPass("");
      showToast("WiFi configuration saved");
      setWifiStatus(await api.getWifiStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    } finally {
      setSaving(null);
    }
  }, [wifiSSID, wifiPass, showToast]);

  const scanWifi = useCallback(async () => {
    setScanning(true);
    try {
      const res = await api.scanWifi();
      setWifiNetworks(res.networks || []);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Scan failed", false);
    } finally {
      setScanning(false);
    }
  }, [showToast]);

  const saveTelegram = useCallback(async () => {
    setSaving("telegram");
    try {
      await api.configureTelegram(tgBotToken, tgChatID, tgOffline, tgQuality);
      setTgBotToken("");
      showToast("Telegram configuration saved");
      setTelegramStatus(await api.getTelegramStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    } finally {
      setSaving(null);
    }
  }, [tgBotToken, tgChatID, tgOffline, tgQuality, showToast]);

  const testTelegram = useCallback(async () => {
    try {
      await api.testTelegram();
      showToast("Test message sent");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Test failed", false);
    }
  }, [showToast]);

  const saveSambaPassword = useCallback(async () => {
    if (!sambaPass) { showToast("Password is required", false); return; }
    if (sambaPass !== sambaPassConfirm) { showToast("Passwords do not match", false); return; }
    setSaving("samba");
    try {
      await api.setSambaPassword(sambaPass);
      setSambaPass("");
      setSambaPassConfirm("");
      showToast("Samba password updated");
      setSambaStatus(await api.getSambaStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    } finally {
      setSaving(null);
    }
  }, [sambaPass, sambaPassConfirm, showToast]);

  const toggleSambaEnabled = useCallback(async (enabled: boolean) => {
    setSaving("samba-toggle");
    try {
      await api.setSambaEnabled(enabled);
      showToast(enabled ? "Samba enabled" : "Samba disabled");
      setSambaStatus(await api.getSambaStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    } finally {
      setSaving(null);
    }
  }, [showToast]);

  useEffect(() => {
    loadConfig();
    loadWebhook();
    loadNetwork();
  }, [loadConfig, loadWebhook, loadNetwork]);

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
          <p className="text-xs text-[var(--color-text-muted)]">
            Uses the Debian <code className="rounded bg-[var(--color-bg-tertiary)] px-1">watchdog</code> daemon (
            <code className="rounded bg-[var(--color-bg-tertiary)] px-1">/etc/watchdog.conf</code>).
          </p>
          <label className={fieldLabel} htmlFor="watchdog-timeout-sec">
            Watchdog timeout (seconds)
          </label>
          <input
            id="watchdog-timeout-sec"
            type="number"
            min={10}
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

      {/* ── Access Point ───────────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Access Point"
          description="Configure the Pi's WiFi hotspot and force mode."
        />
        {apStatus && (
          <div className="mb-4 flex flex-wrap gap-2 text-xs text-[var(--color-text-muted)]">
            <span className={`rounded px-2 py-0.5 font-semibold ${apStatus.active ? "bg-[var(--color-success-bg)] text-[var(--color-success)]" : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"}`}>
              {apStatus.active ? "Active" : "Inactive"}
            </span>
            {apStatus.active && apStatus.client_count > 0 && (
              <span>{apStatus.client_count} client{apStatus.client_count !== 1 ? "s" : ""} connected</span>
            )}
          </div>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className={fieldLabel} htmlFor="ap-ssid">SSID</label>
            <input id="ap-ssid" type="text" className={inputCls} value={apSSID} onChange={(e) => setApSSID(e.target.value)} />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="ap-pass">New passphrase</label>
            <input id="ap-pass" type="password" className={inputCls} value={apPass} onChange={(e) => setApPass(e.target.value)} placeholder="Leave blank to keep current" />
          </div>
        </div>

        {apStatus?.passphrase && (
          <div className="mt-3">
            <label className={fieldLabel}>Current passphrase</label>
            <div className="flex items-center gap-2">
              <input
                type={showApPass ? "text" : "password"}
                className={inputCls}
                value={apStatus.passphrase}
                readOnly
              />
              <button
                type="button"
                onClick={() => setShowApPass((v) => !v)}
                className="shrink-0 rounded border border-[var(--color-border)] px-3 py-2.5 text-sm font-semibold text-[var(--color-text-secondary)] hover:border-[var(--color-accent)]"
              >
                {showApPass ? "Hide" : "Show"}
              </button>
              <button
                type="button"
                onClick={() => copyText(apStatus.passphrase, "Passphrase")}
                className="shrink-0 rounded border border-[var(--color-border)] px-3 py-2.5 text-sm font-semibold text-[var(--color-text-secondary)] hover:border-[var(--color-accent)]"
              >
                Copy
              </button>
            </div>
          </div>
        )}

        <div className="mt-5">
          <button className={btnPrimaryCls} disabled={saving === "ap-basic"} onClick={saveAP}>
            {saving === "ap-basic" ? "Saving…" : "Save SSID / passphrase"}
          </button>
        </div>

        <div className="mt-5">
          <label className={fieldLabel}>Hotspot mode</label>
          <div className="flex flex-wrap gap-2">
            {([
              { mode: "auto", label: "Auto (fallback)", desc: "On only when WiFi/internet is lost" },
              { mode: "on", label: "Always on", desc: "Hotspot stays up at all times" },
              { mode: "off", label: "Always off", desc: "Never start the hotspot" },
            ] as const).map((o) => {
              const isActive = apStatus?.force_mode === o.mode;
              return (
                <button
                  key={o.mode}
                  title={o.desc}
                  onClick={() => forceAP(o.mode)}
                  className={`rounded border px-4 py-2.5 text-sm font-semibold transition-all ${
                    isActive
                      ? "border-[var(--color-accent)] bg-[var(--color-accent)] text-white"
                      : "border-[var(--color-border)] bg-[var(--color-bg-tertiary)] text-[var(--color-text-secondary)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)]"
                  }`}
                >
                  {o.label}
                </button>
              );
            })}
          </div>
          <p className="mt-2 text-xs text-[var(--color-text-muted)]">
            {apStatus?.force_mode === "on" && "Hotspot is forced on at all times."}
            {apStatus?.force_mode === "off" && "Hotspot is forced off — it will not start even when WiFi drops."}
            {(apStatus?.force_mode === "auto" || !apStatus?.force_mode) &&
              "Hotspot starts automatically when the Pi loses WiFi/internet, and stops when it reconnects."}
          </p>
        </div>

        <label className="mt-5 flex items-start gap-3 rounded border border-[var(--color-border)] p-3">
          <input
            type="checkbox"
            className="mt-0.5"
            checked={!!apStatus?.share_internet}
            onChange={(e) => toggleShareInternet(e.target.checked)}
          />
          <span className="text-sm">
            <span className="font-semibold text-[var(--color-text-primary)]">Share internet to clients</span>
            <span className="mt-0.5 block text-xs text-[var(--color-text-muted)]">
              Route devices that join this hotspot (e.g. the car) out to the Pi&apos;s current
              upstream — Bluetooth tethering or WiFi — via NAT. Off keeps the AP an admin-only portal.
              {apStatus?.share_internet && apStatus?.upstream && (
                <span className="text-[var(--color-success)]"> Upstream: {apStatus.upstream}.</span>
              )}
              {apStatus?.share_internet && apStatus?.active && !apStatus?.upstream && (
                <span className="text-[var(--color-danger)]"> No upstream with internet detected.</span>
              )}
            </span>
          </span>
        </label>
      </div>

      {/* ── WiFi Client ─────────────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="WiFi Client"
          description="Connect the Pi to an existing WiFi network."
        />
        {wifiStatus?.connection?.connected && (
          <p className="mb-4 text-sm text-[var(--color-success)]">
            Connected to <strong>{wifiStatus.connection.ssid}</strong>
            {wifiStatus.connection.ip ? ` · ${wifiStatus.connection.ip}` : ""}
          </p>
        )}
        <div className="mb-4">
          <button className={btnPrimaryCls} disabled={scanning} onClick={scanWifi}>
            {scanning ? "Scanning…" : "Scan Networks"}
          </button>
        </div>
        {wifiNetworks.length > 0 && (
          <div className="mb-4 max-h-40 overflow-y-auto rounded border border-[var(--color-border)]">
            {wifiNetworks.map((n) => (
              <button
                key={n.ssid}
                onClick={() => setWifiSSID(n.ssid)}
                className={`flex w-full items-center justify-between px-3 py-2 text-left text-sm transition-colors hover:bg-[var(--color-bg-tertiary)] ${wifiSSID === n.ssid ? "bg-[var(--color-bg-tertiary)]" : ""}`}
              >
                <span className="font-medium text-[var(--color-text-primary)]">{n.ssid}</span>
                <span className="flex items-center gap-3 text-xs text-[var(--color-text-muted)]">
                  <span>{n.security}</span>
                  <span className="tabular-nums">{n.signal}%</span>
                  {n.in_use && <span className="rounded bg-[var(--color-success-bg)] px-1.5 py-0.5 text-[var(--color-success)]">Connected</span>}
                </span>
              </button>
            ))}
          </div>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className={fieldLabel} htmlFor="wifi-ssid">SSID</label>
            <input id="wifi-ssid" type="text" className={inputCls} value={wifiSSID} onChange={(e) => setWifiSSID(e.target.value)} />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="wifi-pass">Password</label>
            <input id="wifi-pass" type="password" className={inputCls} value={wifiPass} onChange={(e) => setWifiPass(e.target.value)} />
          </div>
        </div>
        <div className="mt-5">
          <button className={btnPrimaryCls} disabled={saving === "wifi"} onClick={connectWifi}>
            {saving === "wifi" ? "Connecting…" : "Connect"}
          </button>
        </div>

        {savedWifi.length > 0 && (
          <div className="mt-6 border-t border-[var(--color-border)] pt-4">
            <p className="mb-1 text-sm font-semibold text-[var(--color-text-primary)]">Saved Networks</p>
            <p className="mb-3 text-xs text-[var(--color-text-muted)]">
              Networks this device has joined. Toggle auto-join or forget them.
            </p>
            <div className="divide-y divide-[var(--color-border)] rounded border border-[var(--color-border)]">
              {savedWifi.map((n) => (
                <div key={n.uuid} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                  <span className="flex min-w-0 items-center gap-2">
                    <span className="truncate font-medium text-[var(--color-text-primary)]">{n.name}</span>
                    {n.active && (
                      <span className="shrink-0 rounded bg-[var(--color-success-bg)] px-1.5 py-0.5 text-xs text-[var(--color-success)]">Connected</span>
                    )}
                  </span>
                  <span className="flex shrink-0 items-center gap-3 text-xs">
                    <label className="flex items-center gap-1.5 text-[var(--color-text-muted)]">
                      <input type="checkbox" checked={n.autoconnect} onChange={() => toggleWifiAutoConnect(n)} />
                      Auto-join
                    </label>
                    <button
                      onClick={() => forgetWifi(n.uuid, n.name)}
                      className="rounded border border-[var(--color-border)] px-2 py-1 font-semibold text-[var(--color-text-secondary)] transition-colors hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]"
                    >
                      Forget
                    </button>
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* ── Bluetooth Tethering ─────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Bluetooth Tethering"
          description="Pull internet from a paired phone over Bluetooth (PAN), then share it to the car via the AP's 'Share internet' toggle."
        />
        <div className="mb-4 flex flex-wrap items-center gap-2 text-xs">
          <span className={`rounded px-2 py-0.5 font-semibold ${btStatus?.powered ? "bg-[var(--color-success-bg)] text-[var(--color-success)]" : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"}`}>
            {btStatus?.powered ? "On" : "Off"}
          </span>
          <span className={`rounded px-2 py-0.5 font-semibold ${btStatus?.tethered ? "bg-[var(--color-success-bg)] text-[var(--color-success)]" : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"}`}>
            {btStatus?.tethered ? "Tethered" : "Not tethered"}
          </span>
          {btStatus?.tethered && btStatus?.device_name && (
            <span className="text-[var(--color-text-muted)]">via {btStatus.device_name}</span>
          )}
        </div>

        <div className="mb-4 flex flex-wrap items-center gap-4">
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={!!btStatus?.powered} onChange={(e) => toggleBluetoothPower(e.target.checked)} />
            <span className="text-[var(--color-text-secondary)]">Bluetooth power</span>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={!!btStatus?.discoverable} onChange={(e) => toggleBluetoothDiscoverable(e.target.checked)} disabled={!btStatus?.powered} />
            <span className="text-[var(--color-text-secondary)]">Discoverable / pairable</span>
          </label>
          <button className={btnPrimaryCls} disabled={btScanning || !btStatus?.powered} onClick={scanBluetooth}>
            {btScanning ? "Scanning…" : "Scan"}
          </button>
          <button className="rounded border border-[var(--color-border)] px-3 py-1.5 text-sm font-semibold text-[var(--color-text-secondary)] hover:border-[var(--color-accent)]" onClick={refreshBluetooth}>
            Refresh
          </button>
        </div>

        <p className="mb-3 text-xs text-[var(--color-text-muted)]">
          To tether: turn on Discoverable and pair from your phone (or Scan and Pair a nearby
          device), enable the phone&apos;s Bluetooth tethering, then Connect below.
        </p>

        {btDevices.length === 0 ? (
          <p className="text-sm text-[var(--color-text-muted)]">
            No devices yet. Make the Pi discoverable and pair from your phone, or Scan for nearby devices.
          </p>
        ) : (
          <div className="divide-y divide-[var(--color-border)] rounded border border-[var(--color-border)]">
            {btDevices.map((d) => (
              <div key={d.mac} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
                <span className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-medium text-[var(--color-text-primary)]">{d.name}</span>
                  <span className="shrink-0 text-xs text-[var(--color-text-muted)]">{d.mac}</span>
                  {d.connected && (
                    <span className="shrink-0 rounded bg-[var(--color-success-bg)] px-1.5 py-0.5 text-xs text-[var(--color-success)]">Connected</span>
                  )}
                  {!d.connected && d.paired && (
                    <span className="shrink-0 rounded bg-[var(--color-bg-tertiary)] px-1.5 py-0.5 text-xs text-[var(--color-text-muted)]">Paired</span>
                  )}
                </span>
                <span className="flex shrink-0 items-center gap-1.5">
                  {!d.paired && (
                    <button
                      onClick={() => pairBluetooth(d.mac, d.name)}
                      className="rounded border border-[var(--color-border)] px-2 py-1 text-xs font-semibold text-[var(--color-text-secondary)] transition-colors hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)]"
                    >
                      Pair
                    </button>
                  )}
                  {d.paired && (d.connected ? (
                    <button
                      onClick={() => disconnectBluetooth(d.mac, d.name)}
                      className="rounded border border-[var(--color-border)] px-2 py-1 text-xs font-semibold text-[var(--color-text-secondary)] transition-colors hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]"
                    >
                      Disconnect
                    </button>
                  ) : (
                    <button
                      onClick={() => connectBluetooth(d.mac, d.name)}
                      className="rounded border border-[var(--color-border)] px-2 py-1 text-xs font-semibold text-[var(--color-text-secondary)] transition-colors hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)]"
                    >
                      Connect
                    </button>
                  ))}
                  {d.paired && (
                    <button
                      onClick={() => removeBluetooth(d.mac, d.name)}
                      className="rounded border border-[var(--color-border)] px-2 py-1 text-xs font-semibold text-[var(--color-text-muted)] transition-colors hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]"
                    >
                      Forget
                    </button>
                  )}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ── Telegram Alerts ─────────────────────── */}
      <div className={cardCls}>
        <div className="mb-6 flex items-center justify-between border-b border-[var(--color-border)] pb-3">
          <div>
            <h2 className="text-lg font-bold text-[var(--color-text-primary)]">Telegram Alerts</h2>
            <p className="mt-1 text-sm text-[var(--color-text-muted)]">Send sentry event clips via Telegram bot.</p>
          </div>
          {telegramStatus?.bot_configured && (
            <span className={`inline-flex items-center gap-1 rounded px-2.5 py-0.5 text-xs font-semibold ${telegramStatus.online ? "bg-[var(--color-success-bg)] text-[var(--color-success)]" : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${telegramStatus.online ? "bg-[var(--color-success)]" : "bg-[var(--color-text-muted)]"}`} />
              {telegramStatus.online ? "Online" : "Offline"}
            </span>
          )}
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className={fieldLabel} htmlFor="tg-token">Bot Token</label>
            <input id="tg-token" type="password" className={inputCls} value={tgBotToken} onChange={(e) => setTgBotToken(e.target.value)} placeholder="123456:ABC-DEF…" />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="tg-chat">Chat ID</label>
            <input id="tg-chat" type="text" className={inputCls} value={tgChatID} onChange={(e) => setTgChatID(e.target.value)} placeholder="-1001234567890" />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="tg-offline">Offline Mode</label>
            <select id="tg-offline" className={inputCls} value={tgOffline} onChange={(e) => setTgOffline(e.target.value)}>
              <option value="queue">Queue (send when online)</option>
              <option value="discard">Discard</option>
            </select>
          </div>
          <div>
            <label className={fieldLabel} htmlFor="tg-quality">Video Quality</label>
            <select id="tg-quality" className={inputCls} value={tgQuality} onChange={(e) => setTgQuality(e.target.value)}>
              <option value="hd">HD</option>
              <option value="sd">SD</option>
              <option value="thumbnail">Thumbnail only</option>
            </select>
          </div>
        </div>
        {telegramStatus?.queue_size != null && telegramStatus.queue_size > 0 && (
          <p className="mt-3 text-xs text-[var(--color-text-muted)]">
            {telegramStatus.queue_size} event{telegramStatus.queue_size !== 1 ? "s" : ""} queued
          </p>
        )}
        <div className="mt-5 flex gap-2">
          <button className={btnPrimaryCls} disabled={saving === "telegram"} onClick={saveTelegram}>
            {saving === "telegram" ? "Saving…" : "Save"}
          </button>
          <button
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-5 py-2.5 text-sm font-semibold text-[var(--color-text-secondary)] transition-all hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)] disabled:opacity-50"
            disabled={!telegramStatus?.bot_configured}
            onClick={testTelegram}
          >
            Send Test
          </button>
        </div>
      </div>

      {/* ── Samba File Sharing ──────────────────── */}
      <div className={cardCls}>
        <div className="mb-6 flex items-center justify-between border-b border-[var(--color-border)] pb-3">
          <div>
            <h2 className="text-lg font-bold text-[var(--color-text-primary)]">Samba File Sharing</h2>
            <p className="mt-1 text-sm text-[var(--color-text-muted)]">Access partitions over the network in edit mode.</p>
          </div>
          {sambaStatus && (
            sambaStatus.enabled ? (
              sambaStatus.password_set ? (
                <span className="inline-flex items-center gap-1 rounded bg-[var(--color-success-bg)] px-2.5 py-0.5 text-xs font-semibold text-[var(--color-success)]">
                  <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-success)]" />
                  Configured
                </span>
              ) : (
                <span className="inline-flex items-center gap-1 rounded bg-[var(--color-warning-bg)] px-2.5 py-0.5 text-xs font-semibold text-[var(--color-warning)]">
                  <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-warning)]" />
                  No password
                </span>
              )
            ) : (
              <span className="inline-flex items-center gap-1 rounded bg-[var(--color-bg-tertiary)] px-2.5 py-0.5 text-xs font-semibold text-[var(--color-text-muted)]">
                <span className="h-1.5 w-1.5 rounded-full bg-[var(--color-text-muted)]" />
                Disabled
              </span>
            )
          )}
        </div>
        <label className="mb-4 flex items-center gap-3">
          <input
            type="checkbox"
            checked={!!sambaStatus?.enabled}
            disabled={!sambaStatus || saving === "samba-toggle"}
            onChange={(e) => toggleSambaEnabled(e.target.checked)}
            className="h-4 w-4 rounded accent-[var(--color-accent)]"
          />
          <span className="text-sm text-[var(--color-text-primary)]">
            Enable Samba file sharing
            {saving === "samba-toggle" && (
              <span className="ml-2 text-xs text-[var(--color-text-muted)]">applying…</span>
            )}
          </span>
        </label>
        {sambaStatus && sambaStatus.enabled && sambaStatus.shares.length > 0 && (
          <div className="mb-4 overflow-hidden rounded border border-[var(--color-border)]">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-[var(--color-bg-tertiary)]">
                  <th className="px-3 py-2 text-left font-medium text-[var(--color-text-secondary)]">Share</th>
                  <th className="px-3 py-2 text-left font-medium text-[var(--color-text-secondary)]">Path</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--color-border-light)]">
                {sambaStatus.shares.map((s) => (
                  <tr key={s.name}>
                    <td className="px-3 py-2">
                      <span className="font-medium text-[var(--color-text-primary)]">{s.label}</span>
                      <span className="ml-2 font-mono text-xs text-[var(--color-text-muted)]">\\{s.name}</span>
                    </td>
                    <td className="px-3 py-2 font-mono text-xs text-[var(--color-text-muted)]">{s.path}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <fieldset disabled={!sambaStatus?.enabled} className="contents">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className={fieldLabel} htmlFor="smb-pass">New Password</label>
              <input id="smb-pass" type="password" className={inputCls} value={sambaPass} onChange={(e) => setSambaPass(e.target.value)} placeholder="Enter new Samba password" />
            </div>
            <div>
              <label className={fieldLabel} htmlFor="smb-confirm">Confirm Password</label>
              <input id="smb-confirm" type="password" className={inputCls} value={sambaPassConfirm} onChange={(e) => setSambaPassConfirm(e.target.value)} placeholder="Confirm password" />
            </div>
          </div>
          <div className="mt-5 flex flex-wrap gap-2">
            <button className={btnPrimaryCls} disabled={saving === "samba"} onClick={saveSambaPassword}>
              {saving === "samba" ? "Saving…" : "Set Password"}
            </button>
            <button className="rounded border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-4 py-2.5 text-sm font-semibold text-[var(--color-text-secondary)] transition-all hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)] disabled:opacity-50" onClick={async () => { try { await api.restartSamba(); showToast("Samba services restarted"); } catch (e) { showToast(e instanceof Error ? e.message : "Failed", false); } }}>
              Restart Services
            </button>
            <button className="rounded border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-4 py-2.5 text-sm font-semibold text-[var(--color-text-secondary)] transition-all hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)] disabled:opacity-50" onClick={async () => { try { await api.regenerateSambaConfig(); showToast("Samba config regenerated"); } catch (e) { showToast(e instanceof Error ? e.message : "Failed", false); } }}>
              Regenerate Config
            </button>
          </div>
        </fieldset>
      </div>

      {/* ── Display Preferences ────────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Display Preferences"
          description="UI preferences stored in your browser."
        />
        <div>
          <span className={fieldLabel}>Speed unit</span>
          <div className="mt-2 flex gap-2">
            {(["mph", "kph"] as SpeedUnit[]).map((u) => (
              <button
                key={u}
                onClick={() => setSpeedUnit(u)}
                className={`rounded px-5 py-2 text-sm font-semibold transition-colors ${
                  speedUnit === u
                    ? "bg-[var(--color-accent)] text-white"
                    : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)]"
                }`}
              >
                {u.toUpperCase()}
              </button>
            ))}
          </div>
          <p className="mt-2 text-xs text-[var(--color-text-muted)]">
            Applies to the dashcam HUD telemetry overlay.
          </p>
        </div>

        <div className="mt-5">
          <span className={fieldLabel}>Map tiles</span>
          <div className="mt-2 flex gap-2">
            {(["light", "dark"] as MapTheme[]).map((t) => (
              <button
                key={t}
                onClick={() => setMapTheme(t)}
                className={`rounded px-5 py-2 text-sm font-semibold capitalize transition-colors ${
                  mapTheme === t
                    ? "bg-[var(--color-accent)] text-white"
                    : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)]"
                }`}
              >
                {t}
              </button>
            ))}
          </div>
          <p className="mt-2 text-xs text-[var(--color-text-muted)]">
            Tile style for the in-player GPS map overlay.
          </p>
        </div>
      </div>

      {/* ── Webhook Notifications ──────────────── */}
      <div className={cardCls}>
        <SectionHeader
          title="Webhook Notifications"
          description="POST a JSON payload to any URL when a new Sentry event is detected. Optionally sign payloads with an HMAC-SHA256 secret."
        />
        <div className="space-y-4">
          <label className="flex items-center gap-3">
            <input
              type="checkbox"
              checked={webhookEnabled}
              onChange={(e) => setWebhookEnabled(e.target.checked)}
              className="h-4 w-4 rounded accent-[var(--color-accent)]"
            />
            <span className="text-sm text-[var(--color-text-primary)]">Enable webhook notifications</span>
          </label>
          <div>
            <label className={fieldLabel} htmlFor="webhook-url">
              Webhook URL
            </label>
            <input
              id="webhook-url"
              type="url"
              className={inputCls}
              value={webhookURL}
              onChange={(e) => setWebhookURL(e.target.value)}
              placeholder="https://discord.com/api/webhooks/…"
            />
          </div>
          <div>
            <label className={fieldLabel} htmlFor="webhook-secret">
              Signing secret <span className="normal-case font-normal text-[var(--color-text-muted)]">(optional — leave blank to keep existing)</span>
            </label>
            <input
              id="webhook-secret"
              type="password"
              className={inputCls}
              value={webhookSecret}
              onChange={(e) => setWebhookSecret(e.target.value)}
              placeholder="••••••••"
              autoComplete="new-password"
            />
          </div>
        </div>
        <div className="mt-5 flex justify-end">
          <button
            className={btnPrimaryCls}
            disabled={saving === "webhook"}
            onClick={saveWebhook}
          >
            {saving === "webhook" ? "Saving…" : "Save"}
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
