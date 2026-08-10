"use client";

import { useEffect, useState, useCallback } from "react";
import * as api from "@/lib/api";
import PowerControls from "@/components/PowerControls";
import { EDIT_MODE_CONFIRM } from "@/components/EditModeBanner";
import { StatusBadge, SignalStrength } from "@/components/SystemPanel";
import { useToast } from "@/components/Toast";
import type {
  CompleteAnalytics,
  SystemMetrics,
  AppStatus,
  APStatus,
  WifiStatus,
  GadgetState,
} from "@/lib/types";

function fmt(bytes: number): string {
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
  if (bytes >= 1e6) return `${(bytes / 1e6).toFixed(0)} MB`;
  return `${Math.round(bytes / 1e3)} KB`;
}

const HEALTH_COLOR: Record<string, string> = {
  healthy: "text-[var(--color-success)] bg-[var(--color-success-bg)]",
  caution: "text-[var(--color-warning)] bg-[var(--color-warning-bg)]",
  warning: "text-[var(--color-warning)] bg-[var(--color-warning-bg)]",
  critical: "text-[var(--color-danger)] bg-[var(--color-danger-bg)]",
};

const cardCls = "rounded bg-[var(--color-bg-card)] p-5 shadow-sm";

export default function HomePage() {
  const [analytics, setAnalytics] = useState<CompleteAnalytics | null>(null);
  const [metrics, setMetrics] = useState<SystemMetrics | null>(null);
  const [status, setStatus] = useState<AppStatus | null>(null);
  const [apStatus, setApStatus] = useState<APStatus | null>(null);
  const [wifiStatus, setWifiStatus] = useState<WifiStatus | null>(null);
  const [gadget, setGadget] = useState<GadgetState | null>(null);
  const [switching, setSwitching] = useState(false);
  const [loading, setLoading] = useState(true);
  // Everything below the banner is last-known-good data; without these two the
  // page renders minutes-old numbers as if they were live.
  const [lastSuccess, setLastSuccess] = useState<number | null>(null);
  const [unreachable, setUnreachable] = useState(false);
  const { showToast } = useToast();

  const load = useCallback(async () => {
    if (document.hidden) return;
    // The connectivity card below is `xl:hidden`, and at xl SystemPanel already
    // polls these same three. Each of them forks nmcli/iw on the Pi, so only the
    // side that is actually rendering them pays for them.
    const showsConnectivity = !window.matchMedia("(min-width: 1280px)").matches;
    const [a, m, s, ap, wifi, g] = await Promise.allSettled([
      api.getDashboard(),
      api.getSystemMetrics(),
      api.getStatus(),
      showsConnectivity ? api.getAPStatus() : null,
      showsConnectivity ? api.getWifiStatus() : null,
      showsConnectivity ? api.getGadgetState() : null,
    ]);
    if (a.status === "fulfilled") setAnalytics(a.value);
    if (m.status === "fulfilled") setMetrics(m.value);
    if (s.status === "fulfilled") setStatus(s.value);
    if (ap.status === "fulfilled" && ap.value) setApStatus(ap.value);
    if (wifi.status === "fulfilled" && wifi.value) setWifiStatus(wifi.value);
    if (g.status === "fulfilled" && g.value) setGadget(g.value);

    // The skipped calls settle as fulfilled nulls; counting them would make the
    // device look reachable even when every real request failed.
    const ok = (showsConnectivity ? [a, m, s, ap, wifi, g] : [a, m, s]).some(
      (r) => r.status === "fulfilled",
    );
    if (ok) setLastSuccess(Date.now());
    setUnreachable(!ok);
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, 30_000);
    // A backgrounded tab must not keep waking the Pi, and coming back to it must
    // show fresh data rather than whatever was frozen on screen.
    const onVisibility = () => {
      if (!document.hidden) load();
    };
    // Crossing below xl reveals the connectivity card, whose data the wide-width
    // poll deliberately skipped — refetch now rather than show a false "AP off".
    const mql = window.matchMedia("(min-width: 1280px)");
    const onWidth = () => {
      if (!mql.matches) load();
    };
    document.addEventListener("visibilitychange", onVisibility);
    mql.addEventListener("change", onWidth);
    return () => {
      clearInterval(id);
      document.removeEventListener("visibilitychange", onVisibility);
      mql.removeEventListener("change", onWidth);
    };
  }, [load]);

  const switchMode = useCallback(
    async (mode: "present" | "edit") => {
      if (mode === "edit" && !confirm(EDIT_MODE_CONFIRM)) return;
      setSwitching(true);
      try {
        if (mode === "present") await api.switchToPresent();
        else await api.switchToEdit();
        setStatus(await api.getStatus());
      } catch (e) {
        showToast(e instanceof Error ? e.message : "Mode switch failed", false);
      }
      setSwitching(false);
    },
    [showToast],
  );

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  const health = analytics?.storage_health;
  const ramPct = metrics
    ? (metrics.ram.used_bytes / metrics.ram.total_bytes) * 100
    : 0;

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      {unreachable && (
        <div className="flex items-center justify-between gap-3 rounded border-l-2 border-[var(--color-warning)] bg-[var(--color-warning-bg)] px-4 py-3">
          <p className="text-xs font-semibold text-[var(--color-warning)]">
            Device unreachable
            {lastSuccess && ` — showing data from ${new Date(lastSuccess).toLocaleTimeString()}`}
          </p>
          <button
            onClick={load}
            className="shrink-0 rounded border border-[var(--color-warning)] px-3 py-1.5 text-xs font-semibold text-[var(--color-warning)]"
          >
            Retry
          </button>
        </div>
      )}

      <div>
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Overview</h1>
        {lastSuccess && (
          <p className="mt-1 text-xs text-[var(--color-text-muted)]">
            Updated {new Date(lastSuccess).toLocaleTimeString()}
          </p>
        )}
      </div>

      {/* Health alerts — first, not three phone-screens down */}
      {health?.alerts && health.alerts.length > 0 && (
        <div className={`${cardCls} border-l-2 border-[var(--color-warning)]`}>
          <h2 className="mb-3 text-sm font-semibold text-[var(--color-text-primary)]">
            Health Alerts
          </h2>
          <ul className="space-y-1.5">
            {health.alerts.map((alert, i) => (
              <li key={i} className="text-xs text-[var(--color-text-secondary)]">
                · {alert}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Mode, connectivity and power — hidden on xl where SystemPanel is visible */}
      <div className={`xl:hidden ${cardCls}`}>
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wider text-[var(--color-text-muted)]">
              USB Mode
            </p>
            <p className="mt-0.5 text-sm font-semibold text-[var(--color-text-primary)]">
              {status?.mode_label ?? "—"}
            </p>
          </div>
          <div className="flex gap-2">
            {(["present", "edit"] as const).map((m) => (
              <button
                key={m}
                onClick={() => switchMode(m)}
                disabled={switching || status?.mode === m}
                className={`rounded px-4 py-2 text-xs font-semibold capitalize transition-all disabled:opacity-60 ${
                  status?.mode === m
                    ? m === "present"
                      ? "bg-[var(--color-success)] text-white"
                      : "bg-[var(--color-warning)] text-white"
                    : "border border-[var(--color-border)] text-[var(--color-text-secondary)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)]"
                }`}
              >
                {m}
              </button>
            ))}
          </div>
        </div>
        <p className="mt-2 text-[10px] text-[var(--color-text-muted)]">
          Present: car records · Edit: manage files
        </p>

        {/* Connectivity: an unbound gadget with mode=present is the difference
            between "recording" and "the car cannot see the drive". */}
        <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-[var(--color-border-light)] pt-3">
          <StatusBadge
            active={!!apStatus?.active}
            activeLabel={`AP: ${apStatus?.ssid || "on"}`}
            inactiveLabel="AP off"
          />
          <StatusBadge
            active={!!wifiStatus?.connection?.connected}
            activeLabel={`WiFi: ${wifiStatus?.connection?.ssid || "on"}`}
            inactiveLabel="WiFi off"
          />
          {wifiStatus?.connection?.connected && wifiStatus.connection.signal != null && (
            <SignalStrength rssi={wifiStatus.connection.signal} />
          )}
          <StatusBadge
            active={!!gadget?.gadget_present}
            activeLabel="USB bound"
            inactiveLabel="USB unbound"
          />
        </div>

        <div className="mt-4 border-t border-[var(--color-border-light)] pt-3">
          <span className="text-xs font-medium tracking-wider text-[var(--color-text-muted)]">
            Power
          </span>
          <PowerControls />
        </div>
      </div>

      {/* Storage */}
      {analytics?.partition_usage && analytics.partition_usage.length > 0 && (
        <div className={cardCls}>
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-[var(--color-text-primary)]">Storage</h2>
            {health && (
              <span className={`rounded px-2 py-0.5 text-[10px] font-semibold capitalize ${HEALTH_COLOR[health.status]}`}>
                {health.status}
              </span>
            )}
          </div>
          <div className="space-y-4">
            {analytics.partition_usage.map((p) => (
              <div key={p.name}>
                <div className="mb-1.5 flex items-center justify-between text-xs">
                  <span className="font-medium text-[var(--color-text-primary)]">{p.label}</span>
                  <span className="tabular-nums text-[var(--color-text-muted)]">
                    {fmt(p.used_bytes)} / {fmt(p.total_bytes)}
                  </span>
                </div>
                <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-bg-tertiary)]">
                  <div
                    className={`h-full rounded-full transition-all ${
                      p.percent_used > 90
                        ? "bg-[var(--color-danger)]"
                        : p.percent_used > 75
                          ? "bg-[var(--color-warning)]"
                          : "bg-[var(--color-accent)]"
                    }`}
                    style={{ width: `${Math.min(100, p.percent_used)}%` }}
                  />
                </div>
                <p className="mt-0.5 text-right text-[10px] tabular-nums text-[var(--color-text-muted)]">
                  {p.percent_used.toFixed(0)}% · {fmt(p.free_bytes)} free
                </p>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Clips + System side by side */}
      <div className="grid gap-6 sm:grid-cols-2">
        {analytics?.video_statistics && analytics.video_statistics.length > 0 && (
          <div className={cardCls}>
            <h2 className="mb-4 text-sm font-semibold text-[var(--color-text-primary)]">Clips</h2>
            <div className="divide-y divide-[var(--color-border-light)]">
              {analytics.video_statistics.map((s) => (
                <div key={s.folder} className="flex items-center justify-between py-2 text-xs">
                  <span className="text-[var(--color-text-secondary)]">{s.folder}</span>
                  <span className="tabular-nums text-[var(--color-text-muted)]">
                    {s.count} clip{s.count !== 1 ? "s" : ""} · {fmt(s.size_bytes)}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}

        {metrics && (
          <div className={cardCls}>
            <h2 className="mb-4 text-sm font-semibold text-[var(--color-text-primary)]">System</h2>
            <div className="space-y-4">
              <UsageBar
                label="CPU"
                pct={metrics.cpu.usage_percent}
                suffix={metrics.cpu.temp_c != null ? `${metrics.cpu.temp_c.toFixed(0)}°C` : undefined}
              />
              <UsageBar
                label="RAM"
                pct={ramPct}
                suffix={`${fmt(metrics.ram.used_bytes)} / ${fmt(metrics.ram.total_bytes)}`}
              />
              {metrics.power.watts != null && (
                <div className="flex items-center justify-between text-xs">
                  <span className="text-[var(--color-text-secondary)]">Power</span>
                  <span className="tabular-nums text-[var(--color-text-muted)]">
                    {metrics.power.watts.toFixed(1)} W
                  </span>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function UsageBar({
  label,
  pct,
  suffix,
}: {
  label: string;
  pct: number;
  suffix?: string;
}) {
  return (
    <div>
      <div className="mb-1 flex items-center justify-between text-xs">
        <span className="text-[var(--color-text-secondary)]">{label}</span>
        <span className="tabular-nums text-[var(--color-text-muted)]">
          {suffix ?? `${pct.toFixed(0)}%`}
        </span>
      </div>
      <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-bg-tertiary)]">
        {/* Same 90/75 thresholds as the storage bars above — two bars on one page
            disagreeing on "red" is worse than either threshold being wrong. */}
        <div
          className={`h-full rounded-full transition-all ${
            pct > 90
              ? "bg-[var(--color-danger)]"
              : pct > 75
                ? "bg-[var(--color-warning)]"
                : "bg-[var(--color-accent)]"
          }`}
          style={{ width: `${Math.min(100, pct)}%` }}
        />
      </div>
    </div>
  );
}
