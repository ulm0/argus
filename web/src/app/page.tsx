"use client";

import { useEffect, useState, useCallback } from "react";
import * as api from "@/lib/api";
import type {
  APStatus,
  SambaStatus,
  WifiNetwork,
  WifiStatus,
  TelegramStatus,
} from "@/lib/types";

const inputCls =
  "mt-1 block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-3 py-2.5 text-sm text-[var(--color-text-primary)] shadow-sm transition-all placeholder:text-[var(--color-text-muted)] focus:border-[var(--color-accent)] focus:outline-none focus:ring-2 focus:ring-[var(--color-accent)]/20";

const cardCls =
  "rounded bg-[var(--color-bg-card)] p-6 shadow-sm";

const btnPrimaryCls =
  "rounded bg-[var(--color-accent)] px-5 py-2.5 text-sm font-semibold text-white transition-all hover:bg-[var(--color-accent-hover)] active:scale-[0.98]";

const btnSecondaryCls =
  "rounded border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-5 py-2.5 text-sm font-semibold text-[var(--color-text-secondary)] transition-all hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)]";

export default function HomePage() {
  const [apStatus, setApStatus] = useState<APStatus | null>(null);
  const [wifiStatus, setWifiStatus] = useState<WifiStatus | null>(null);
  const [wifiNetworks, setWifiNetworks] = useState<WifiNetwork[]>([]);
  const [telegramStatus, setTelegramStatus] = useState<TelegramStatus | null>(null);
  const [sambaStatus, setSambaStatus] = useState<SambaStatus | null>(null);

  const [apSSID, setApSSID] = useState("");
  const [apPass, setApPass] = useState("");
  const [wifiSSID, setWifiSSID] = useState("");
  const [wifiPass, setWifiPass] = useState("");
  const [scanning, setScanning] = useState(false);

  const [tgBotToken, setTgBotToken] = useState("");
  const [tgChatID, setTgChatID] = useState("");
  const [tgOffline, setTgOffline] = useState("queue");
  const [tgQuality, setTgQuality] = useState("hd");
  const [sambaPass, setSambaPass] = useState("");
  const [sambaPassConfirm, setSambaPassConfirm] = useState("");

  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const [loading, setLoading] = useState(true);

  const showToast = useCallback((msg: string, ok = true) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadAll = useCallback(async () => {
    try {
      const [ap, wifi, tg, smb] = await Promise.all([
        api.getAPStatus(),
        api.getWifiStatus(),
        api.getTelegramStatus(),
        api.getSambaStatus(),
      ]);
      setApStatus(ap);
      setApSSID(ap.ssid || "");
      setWifiStatus(wifi);
      if (wifi.connection?.ssid) setWifiSSID(wifi.connection.ssid);
      setTelegramStatus(tg);
      setSambaStatus(smb);
    } catch {
      showToast("Failed to load status", false);
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    loadAll();
  }, [loadAll]);

  const handleAPConfig = async () => {
    try {
      await api.configureAP({ ssid: apSSID, passphrase: apPass });
      showToast("AP configuration saved");
      setApStatus(await api.getAPStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleAPForce = async (mode: "auto" | "on" | "off") => {
    try {
      await api.forceAP(mode);
      showToast(`AP force mode: ${mode}`);
      setApStatus(await api.getAPStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleWifiScan = async () => {
    setScanning(true);
    try {
      const res = await api.scanWifi();
      setWifiNetworks(res.networks || []);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Scan failed", false);
    } finally {
      setScanning(false);
    }
  };

  const handleWifiConfig = async () => {
    try {
      await api.configureWifi(wifiSSID, wifiPass);
      showToast("WiFi configuration saved");
      setWifiStatus(await api.getWifiStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleTelegramConfig = async () => {
    try {
      await api.configureTelegram(tgBotToken, tgChatID, tgOffline, tgQuality);
      showToast("Telegram configuration saved");
      setTelegramStatus(await api.getTelegramStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleTelegramTest = async () => {
    try {
      await api.testTelegram();
      showToast("Test message sent");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Test failed", false);
    }
  };

  const handleSambaPassword = async () => {
    if (!sambaPass) {
      showToast("Password is required", false);
      return;
    }
    if (sambaPass !== sambaPassConfirm) {
      showToast("Passwords do not match", false);
      return;
    }
    try {
      await api.setSambaPassword(sambaPass);
      showToast("Samba password updated");
      setSambaPass("");
      setSambaPassConfirm("");
      setSambaStatus(await api.getSambaStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleSambaRestart = async () => {
    try {
      await api.restartSamba();
      showToast("Samba services restarted");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Restart failed", false);
    }
  };

  const handleSambaRegenerate = async () => {
    try {
      await api.regenerateSambaConfig();
      showToast("Samba config regenerated and restarted");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Regenerate failed", false);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      {toast && (
        <div
          className={`fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-sm px-4 py-3 text-sm font-medium shadow-lg transition-all ${
            toast.ok ? "bg-[var(--color-success)] text-white" : "bg-[var(--color-danger)] text-white"
          }`}
        >
          {toast.msg}
        </div>
      )}

      {/* Greeting */}
      <div>
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">
          Dashboard
        </h1>
        <p className="mt-1 text-sm text-[var(--color-text-muted)]">
          Configure your Argus network and notification settings.
        </p>
      </div>

      {/* Access Point */}
      <section className={cardCls}>
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">
          Access Point
        </h2>
        <div className="mt-4 space-y-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                SSID
              </label>
              <input
                type="text"
                value={apSSID}
                onChange={(e) => setApSSID(e.target.value)}
                className={inputCls}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                Passphrase
              </label>
              <input
                type="password"
                value={apPass}
                onChange={(e) => setApPass(e.target.value)}
                className={inputCls}
              />
            </div>
          </div>
          <div className="flex flex-wrap gap-3">
            <button onClick={handleAPConfig} className={btnPrimaryCls}>
              Save AP Config
            </button>
            <button onClick={() => handleAPForce("on")} className={btnSecondaryCls}>
              Force On
            </button>
            <button onClick={() => handleAPForce("off")} className={btnSecondaryCls}>
              Force Off
            </button>
            <button onClick={() => handleAPForce("auto")} className={btnSecondaryCls}>
              Auto
            </button>
          </div>
        </div>
      </section>

      {/* WiFi Client */}
      <section className={cardCls}>
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">
          WiFi Client
        </h2>
        {wifiStatus?.connection?.connected && (
          <p className="mt-1 text-sm text-[var(--color-success)]">
            Connected to <strong>{wifiStatus.connection.ssid}</strong>
            {wifiStatus.connection.ip ? ` (${wifiStatus.connection.ip})` : ""}
          </p>
        )}
        <div className="mt-4 space-y-4">
          <div className="flex gap-3">
            <button
              onClick={handleWifiScan}
              disabled={scanning}
              className={`${btnPrimaryCls} disabled:opacity-50`}
            >
              {scanning ? "Scanning..." : "Scan Networks"}
            </button>
          </div>
          {wifiNetworks.length > 0 && (
            <div className="max-h-40 overflow-y-auto rounded-sm border border-[var(--color-border)]">
              {wifiNetworks.map((n) => (
                <button
                  key={n.ssid}
                  onClick={() => setWifiSSID(n.ssid)}
                  className={`flex w-full items-center justify-between px-3 py-2 text-left text-sm transition-colors hover:bg-[var(--color-bg-tertiary)] ${
                    wifiSSID === n.ssid ? "bg-[var(--color-accent-subtle)]" : ""
                  }`}
                >
                  <span className="font-medium text-[var(--color-text-primary)]">
                    {n.ssid}
                  </span>
                  <span className="flex items-center gap-2 text-xs text-[var(--color-text-muted)]">
                    <span>{n.security}</span>
                    <span className="tabular-nums">{n.signal}%</span>
                    {n.in_use && (
                      <span className="rounded bg-[var(--color-success-bg)] px-1.5 py-0.5 text-[var(--color-success)]">
                        Connected
                      </span>
                    )}
                  </span>
                </button>
              ))}
            </div>
          )}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                SSID
              </label>
              <input
                type="text"
                value={wifiSSID}
                onChange={(e) => setWifiSSID(e.target.value)}
                className={inputCls}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                Password
              </label>
              <input
                type="password"
                value={wifiPass}
                onChange={(e) => setWifiPass(e.target.value)}
                className={inputCls}
              />
            </div>
          </div>
          <button onClick={handleWifiConfig} className={btnPrimaryCls}>
            Connect
          </button>
        </div>
      </section>

      {/* Telegram */}
      <section className={cardCls}>
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">
            Telegram Alerts
          </h2>
          {telegramStatus?.bot_configured && (
            <span
              className={`inline-flex items-center rounded px-2.5 py-0.5 text-xs font-semibold ${
                telegramStatus.online
                  ? "bg-[var(--color-success-bg)] text-[var(--color-success)]"
                  : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"
              }`}
            >
              {telegramStatus.online ? "Online" : "Offline"}
            </span>
          )}
        </div>
        <div className="mt-4 space-y-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                Bot Token
              </label>
              <input
                type="password"
                value={tgBotToken}
                onChange={(e) => setTgBotToken(e.target.value)}
                placeholder="123456:ABC-DEF..."
                className={inputCls}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                Chat ID
              </label>
              <input
                type="text"
                value={tgChatID}
                onChange={(e) => setTgChatID(e.target.value)}
                placeholder="-1001234567890"
                className={inputCls}
              />
            </div>
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                Offline Mode
              </label>
              <select
                value={tgOffline}
                onChange={(e) => setTgOffline(e.target.value)}
                className={inputCls}
              >
                <option value="queue">Queue (send when online)</option>
                <option value="discard">Discard</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                Video Quality
              </label>
              <select
                value={tgQuality}
                onChange={(e) => setTgQuality(e.target.value)}
                className={inputCls}
              >
                <option value="hd">HD</option>
                <option value="sd">SD</option>
                <option value="thumbnail">Thumbnail only</option>
              </select>
            </div>
          </div>
          <div className="flex gap-3">
            <button onClick={handleTelegramConfig} className={btnPrimaryCls}>
              Save
            </button>
            <button
              onClick={handleTelegramTest}
              disabled={!telegramStatus?.bot_configured}
              className={`${btnSecondaryCls} disabled:opacity-50`}
            >
              Send Test
            </button>
          </div>
        </div>
      </section>

      {/* Samba */}
      <section className={cardCls}>
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">
            Samba File Sharing
          </h2>
          {sambaStatus?.password_set && (
            <span className="inline-flex items-center rounded px-2.5 py-0.5 text-xs font-semibold bg-[var(--color-success-bg)] text-[var(--color-success)]">
              Configured
            </span>
          )}
        </div>
        {sambaStatus && (
          <p className="mt-1 text-sm text-[var(--color-text-muted)]">
            User: <span className="font-medium text-[var(--color-text-secondary)]">{sambaStatus.user}</span>
            {" \u00b7 "}
            Config: <span className="font-mono text-xs text-[var(--color-text-secondary)]">{sambaStatus.config_path}</span>
          </p>
        )}
        <div className="mt-4 space-y-4">
          {sambaStatus && sambaStatus.shares.length > 0 && (
            <div className="overflow-hidden rounded-sm border border-[var(--color-border)]">
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
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                New Password
              </label>
              <input
                type="password"
                value={sambaPass}
                onChange={(e) => setSambaPass(e.target.value)}
                placeholder="Enter new Samba password"
                className={inputCls}
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-[var(--color-text-secondary)]">
                Confirm Password
              </label>
              <input
                type="password"
                value={sambaPassConfirm}
                onChange={(e) => setSambaPassConfirm(e.target.value)}
                placeholder="Confirm password"
                className={inputCls}
              />
            </div>
          </div>
          <div className="flex flex-wrap gap-3">
            <button onClick={handleSambaPassword} className={btnPrimaryCls}>
              Set Password
            </button>
            <button onClick={handleSambaRestart} className={btnSecondaryCls}>
              Restart Services
            </button>
            <button onClick={handleSambaRegenerate} className={btnSecondaryCls}>
              Regenerate Config
            </button>
          </div>
        </div>
      </section>
    </div>
  );
}
