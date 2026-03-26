"use client";

import { useCallback, useEffect, useState } from "react";
import * as api from "@/lib/api";
import type {
  AppStatus,
  APStatus,
  WifiStatus,
  GadgetState,
  SambaStatus,
  TelegramStatus,
  ModeToken,
} from "@/lib/types";

function ModeBadge({ mode, label }: { mode: ModeToken; label: string }) {
  const styles: Record<ModeToken, string> = {
    present: "bg-[var(--color-success-bg)] text-[var(--color-success)]",
    edit: "bg-[var(--color-warning-bg)] text-[var(--color-warning)]",
    unknown: "bg-[var(--color-bg-tertiary)] text-[var(--color-text-secondary)]",
  };

  const dot: Record<ModeToken, string> = {
    present: "bg-[var(--color-success)]",
    edit: "bg-[var(--color-warning)]",
    unknown: "bg-[var(--color-text-muted)]",
  };

  return (
    <span className={`inline-flex items-center rounded px-2.5 py-1 text-xs font-semibold ${styles[mode]}`}>
      <span className={`mr-1.5 h-1.5 w-1.5 rounded-full ${dot[mode]}`} />
      {label}
    </span>
  );
}

function StatusBadge({ active, activeLabel, inactiveLabel }: { active: boolean; activeLabel: string; inactiveLabel: string }) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-[10px] font-semibold ${
        active
          ? "bg-[var(--color-success-bg)] text-[var(--color-success)]"
          : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"
      }`}
    >
      <span className={`h-1 w-1 rounded-full ${active ? "bg-[var(--color-success)]" : "bg-[var(--color-text-muted)]"}`} />
      {active ? activeLabel : inactiveLabel}
    </span>
  );
}

export default function SystemPanel() {
  const [status, setStatus] = useState<AppStatus | null>(null);
  const [apStatus, setApStatus] = useState<APStatus | null>(null);
  const [wifiStatus, setWifiStatus] = useState<WifiStatus | null>(null);
  const [gadget, setGadget] = useState<GadgetState | null>(null);
  const [telegram, setTelegram] = useState<TelegramStatus | null>(null);
  const [samba, setSamba] = useState<SambaStatus | null>(null);
  const [switching, setSwitching] = useState(false);

  const loadAll = useCallback(async () => {
    const results = await Promise.allSettled([
      api.getStatus(),
      api.getAPStatus(),
      api.getWifiStatus(),
      api.getGadgetState(),
      api.getTelegramStatus(),
      api.getSambaStatus(),
    ]);
    if (results[0].status === "fulfilled") setStatus(results[0].value);
    if (results[1].status === "fulfilled") setApStatus(results[1].value);
    if (results[2].status === "fulfilled") setWifiStatus(results[2].value);
    if (results[3].status === "fulfilled") setGadget(results[3].value);
    if (results[4].status === "fulfilled") setTelegram(results[4].value);
    if (results[5].status === "fulfilled") setSamba(results[5].value);
  }, []);

  useEffect(() => {
    loadAll();
    const id = setInterval(loadAll, 15000);
    return () => clearInterval(id);
  }, [loadAll]);

  const handleModeSwitch = async (mode: "present" | "edit") => {
    setSwitching(true);
    try {
      if (mode === "present") await api.switchToPresent();
      else await api.switchToEdit();
      const s = await api.getStatus();
      setStatus(s);
    } catch {
      // silently ignore
    } finally {
      setSwitching(false);
    }
  };

  return (
    <aside className="hidden w-72 shrink-0 overflow-y-auto border-l border-[var(--color-border)] bg-[var(--color-bg-primary)] xl:block">
      <div className="p-5">
        <h2 className="text-xs font-semibold tracking-wider text-[var(--color-text-muted)]">
          System Status
        </h2>

        {/* Mode */}
        <div className="mt-5 rounded bg-[var(--color-bg-card-nested)] p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
              USB Mode
            </span>
            {status && <ModeBadge mode={status.mode} label={status.mode_label} />}
          </div>
          <div className="mt-3 flex gap-2">
            <button
              onClick={() => handleModeSwitch("present")}
              disabled={switching || status?.mode === "present"}
              className={`flex-1 rounded px-3 py-2 text-xs font-semibold transition-all ${
                status?.mode === "present"
                  ? "bg-[var(--color-success)] text-white"
                  : "border border-[var(--color-border)] bg-[var(--color-bg-card)] text-[var(--color-text-secondary)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)]"
              }`}
            >
              {switching ? "..." : "Present"}
            </button>
            <button
              onClick={() => handleModeSwitch("edit")}
              disabled={switching || status?.mode === "edit"}
              className={`flex-1 rounded px-3 py-2 text-xs font-semibold transition-all ${
                status?.mode === "edit"
                  ? "bg-[var(--color-warning)] text-white"
                  : "border border-[var(--color-border)] bg-[var(--color-bg-card)] text-[var(--color-text-secondary)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)]"
              }`}
            >
              {switching ? "..." : "Edit"}
            </button>
          </div>
        </div>

        {/* Hostname */}
        {status?.hostname && (
          <div className="mt-4 rounded bg-[var(--color-bg-card-nested)] p-4">
            <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
              Hostname
            </span>
            <p className="mt-1 text-sm font-semibold text-[var(--color-text-primary)]">
              {status.hostname}
            </p>
          </div>
        )}

        {/* AP Status */}
        <div className="mt-4 rounded bg-[var(--color-bg-card-nested)] p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
              Access Point
            </span>
            <StatusBadge active={!!apStatus?.active} activeLabel="Active" inactiveLabel="Inactive" />
          </div>
          {apStatus?.active && (
            <div className="mt-2 space-y-1">
              {apStatus.ssid && (
                <p className="text-xs text-[var(--color-text-secondary)]">
                  SSID: <span className="font-medium text-[var(--color-text-primary)]">{apStatus.ssid}</span>
                </p>
              )}
              {apStatus.client_count > 0 && (
                <p className="text-xs text-[var(--color-text-secondary)]">
                  {apStatus.client_count} client{apStatus.client_count > 1 ? "s" : ""} connected
                </p>
              )}
            </div>
          )}
        </div>

        {/* WiFi Status */}
        <div className="mt-4 rounded bg-[var(--color-bg-card-nested)] p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
              WiFi
            </span>
            <StatusBadge active={!!wifiStatus?.connection?.connected} activeLabel="Connected" inactiveLabel="Disconnected" />
          </div>
          {wifiStatus?.connection?.connected && (
            <div className="mt-2 space-y-1">
              {wifiStatus.connection.ssid && (
                <p className="text-xs text-[var(--color-text-secondary)]">
                  <span className="font-medium text-[var(--color-text-primary)]">{wifiStatus.connection.ssid}</span>
                </p>
              )}
              {wifiStatus.connection.ip && (
                <p className="font-mono text-[10px] text-[var(--color-text-muted)]">{wifiStatus.connection.ip}</p>
              )}
              {wifiStatus.connection.signal != null && (
                <div className="flex items-center gap-1.5">
                  <div className="flex items-end gap-0.5">
                    {[25, 50, 75, 100].map((threshold) => (
                      <div
                        key={threshold}
                        className={`w-1 rounded-full ${
                          (wifiStatus.connection.signal ?? 0) >= threshold
                            ? "bg-[var(--color-accent)]"
                            : "bg-[var(--color-border)]"
                        }`}
                        style={{ height: `${threshold / 10 + 4}px` }}
                      />
                    ))}
                  </div>
                  <span className="text-[10px] tabular-nums text-[var(--color-text-muted)]">{wifiStatus.connection.signal}%</span>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Gadget State */}
        {gadget && (
          <div className="mt-4 rounded bg-[var(--color-bg-card-nested)] p-4">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
                USB Gadget
              </span>
              <StatusBadge active={gadget.gadget_present} activeLabel="Bound" inactiveLabel="Unbound" />
            </div>
          </div>
        )}

        {/* Telegram */}
        {telegram?.bot_configured && (
          <div className="mt-4 rounded bg-[var(--color-bg-card-nested)] p-4">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
                Telegram
              </span>
              <StatusBadge active={telegram.online} activeLabel="Online" inactiveLabel="Offline" />
            </div>
            {telegram.queue_size > 0 && (
              <p className="mt-2 text-xs text-[var(--color-text-secondary)]">
                {telegram.queue_size} queued event{telegram.queue_size > 1 ? "s" : ""}
              </p>
            )}
          </div>
        )}

        {/* Samba */}
        <div className="mt-4 rounded bg-[var(--color-bg-card-nested)] p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
              Samba
            </span>
            <StatusBadge active={!!samba?.password_set} activeLabel="Configured" inactiveLabel="Not Set" />
          </div>
          {samba && (
            <p className="mt-2 text-xs text-[var(--color-text-secondary)]">
              {samba.shares.length} share{samba.shares.length !== 1 ? "s" : ""}
            </p>
          )}
        </div>
      </div>
    </aside>
  );
}
