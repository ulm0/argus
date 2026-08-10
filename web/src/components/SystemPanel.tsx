"use client";

import { useCallback, useEffect, useState } from "react";
import * as api from "@/lib/api";
import PowerControls from "@/components/PowerControls";
import { EDIT_MODE_CONFIRM } from "@/components/EditModeBanner";
import { useToast } from "@/components/Toast";
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

// SignalStrength renders a 4-bar WiFi indicator colored by RSSI bucket.
//   ≥ -55 dBm → 4 bars, green   (excellent)
//   -55 .. -65 → 3 bars, green   (good)
//   -65 .. -75 → 2 bars, yellow  (fair)
//   -75 .. -85 → 1 bar,  red     (weak)
//   < -85      → 0 bars, red     (no usable signal)
export function SignalStrength({ rssi }: { rssi: number }) {
  let bars: number;
  let color: string;
  if (rssi >= -55) {
    bars = 4;
    color = "var(--color-success)";
  } else if (rssi >= -65) {
    bars = 3;
    color = "var(--color-success)";
  } else if (rssi >= -75) {
    bars = 2;
    color = "var(--color-warning)";
  } else if (rssi >= -85) {
    bars = 1;
    color = "var(--color-danger)";
  } else {
    bars = 0;
    color = "var(--color-danger)";
  }

  return (
    <div className="flex items-center gap-1.5" title={`Signal: ${rssi} dBm`}>
      <div className="flex items-end gap-0.5">
        {[1, 2, 3, 4].map((step) => (
          <div
            key={step}
            className="w-1 rounded-full"
            style={{
              height: `${step * 2.5 + 3}px`,
              backgroundColor: step <= bars ? color : "var(--color-border)",
            }}
          />
        ))}
      </div>
      <span className="text-[10px] tabular-nums text-[var(--color-text-muted)]">{rssi} dBm</span>
    </div>
  );
}

export function StatusBadge({ active, activeLabel, inactiveLabel }: { active: boolean; activeLabel: string; inactiveLabel: string }) {
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
  // Two consecutive all-failed polls means everything on screen is older than
  // two poll intervals — derived from the polling already in place.
  const [failedPolls, setFailedPolls] = useState(0);
  const { showToast } = useToast();

  const loadAll = useCallback(async () => {
    if (document.hidden) return;
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
    setFailedPolls((n) => (results.some((r) => r.status === "fulfilled") ? 0 : n + 1));
  }, []);

  useEffect(() => {
    // The panel is `hidden ... xl:block`, so it is display:none below 1280px.
    // Only poll while it can actually be visible to avoid waking the Pi's HTTP
    // stack (nmcli/iw forks) and the phone radio to render nothing.
    const mql = window.matchMedia("(min-width: 1280px)");
    let id: ReturnType<typeof setInterval> | null = null;

    const sync = () => {
      if (mql.matches) {
        if (id === null) {
          loadAll();
          id = setInterval(loadAll, 15000);
        }
      } else if (id !== null) {
        clearInterval(id);
        id = null;
      }
    };

    // Returning to a backgrounded tab must show fresh data, not whatever was on
    // screen when it was hidden (loadAll no-ops while hidden).
    const onVisibility = () => {
      if (!document.hidden && mql.matches) loadAll();
    };

    sync();
    mql.addEventListener("change", sync);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      mql.removeEventListener("change", sync);
      document.removeEventListener("visibilitychange", onVisibility);
      if (id !== null) clearInterval(id);
    };
  }, [loadAll]);

  const handleModeSwitch = async (mode: "present" | "edit") => {
    if (mode === "edit" && !confirm(EDIT_MODE_CONFIRM)) return;
    setSwitching(true);
    try {
      if (mode === "present") await api.switchToPresent();
      else await api.switchToEdit();
      const s = await api.getStatus();
      setStatus(s);
    } catch (e) {
      // A silent failure leaves the badge showing a mode the device is not in —
      // the user drives off believing Sentry is recording.
      showToast(e instanceof Error ? e.message : "Mode switch failed", false);
    } finally {
      setSwitching(false);
    }
  };

  return (
    <aside className="hidden w-72 shrink-0 overflow-y-auto border-l border-[var(--color-border)] bg-[var(--color-bg-primary)] xl:block">
      <div className="p-5">
        <div className="flex items-center justify-between">
          <h2 className="text-xs font-semibold tracking-wider text-[var(--color-text-muted)]">
            System Status
          </h2>
          {failedPolls >= 2 && (
            <span className="rounded bg-[var(--color-warning-bg)] px-2 py-0.5 text-[10px] font-semibold text-[var(--color-warning)]">
              Data stale
            </span>
          )}
        </div>

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
          <p className="mt-2 text-[10px] text-[var(--color-text-muted)]">
            Present: car records · Edit: manage files
          </p>
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
                <SignalStrength rssi={wifiStatus.connection.signal} />
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
            {samba && !samba.enabled ? (
              <StatusBadge active={false} activeLabel="Disabled" inactiveLabel="Disabled" />
            ) : (
              <StatusBadge active={!!samba?.password_set} activeLabel="Configured" inactiveLabel="Not Set" />
            )}
          </div>
          {samba && samba.enabled && (
            <p className="mt-2 text-xs text-[var(--color-text-secondary)]">
              {samba.shares.length} share{samba.shares.length !== 1 ? "s" : ""}
            </p>
          )}
        </div>

        {/* Power */}
        <div className="mt-4 rounded bg-[var(--color-bg-card-nested)] p-4">
          <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
            Power
          </span>
          <PowerControls />
        </div>
      </div>
    </aside>
  );
}
