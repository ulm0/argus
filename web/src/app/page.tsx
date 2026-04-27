"use client";

import { useEffect, useState, useCallback } from "react";
import * as api from "@/lib/api";
import type { CompleteAnalytics, SystemMetrics, AppStatus } from "@/lib/types";

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
  const [switching, setSwitching] = useState(false);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    const [a, m, s] = await Promise.allSettled([
      api.getDashboard(),
      api.getSystemMetrics(),
      api.getStatus(),
    ]);
    if (a.status === "fulfilled") setAnalytics(a.value);
    if (m.status === "fulfilled") setMetrics(m.value);
    if (s.status === "fulfilled") setStatus(s.value);
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, [load]);

  const switchMode = useCallback(async (mode: "present" | "edit") => {
    setSwitching(true);
    try {
      if (mode === "present") await api.switchToPresent();
      else await api.switchToEdit();
      setStatus(await api.getStatus());
    } catch {}
    setSwitching(false);
  }, []);

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
      <div>
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Overview</h1>
        {analytics?.last_updated && (
          <p className="mt-1 text-xs text-[var(--color-text-muted)]">
            Updated {new Date(analytics.last_updated).toLocaleTimeString()}
          </p>
        )}
      </div>

      {/* Mode control — hidden on xl where SystemPanel is visible */}
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

      {/* Health alerts */}
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
        <div
          className={`h-full rounded-full transition-all ${
            pct > 85
              ? "bg-[var(--color-danger)]"
              : pct > 65
                ? "bg-[var(--color-warning)]"
                : "bg-[var(--color-accent)]"
          }`}
          style={{ width: `${Math.min(100, pct)}%` }}
        />
      </div>
    </div>
  );
}
