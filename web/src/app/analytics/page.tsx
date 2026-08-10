"use client";

import { useEffect, useState, useCallback } from "react";
import * as api from "@/lib/api";
import type {
  CompleteAnalytics,
  PartitionUsage,
  VideoStats,
  FsckMode,
  FsckStatus,
  FsckCheckResult,
  SystemMetrics,
} from "@/lib/types";
import { useToast } from "@/components/Toast";

// System metrics are cheap and change constantly; everything else on this page
// is a disk walk, so only the metrics are polled.
const METRICS_POLL_MS = 10_000;

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function formatDate(ts: string): string {
  if (!ts) return "—";
  try {
    return new Date(ts).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
  } catch {
    return ts;
  }
}

function formatCpuCapacity(mhz?: number): string {
  if (!mhz || mhz <= 0) return "N/A";
  if (mhz >= 1000) return `${(mhz / 1000).toFixed(1)} GHz`;
  return `${mhz.toFixed(0)} MHz`;
}

const HEALTH_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  healthy: { bg: "bg-[var(--color-success-bg)]", text: "text-[var(--color-success)]", dot: "bg-[var(--color-success)]" },
  caution: { bg: "bg-[var(--color-warning-bg)]", text: "text-[var(--color-warning)]", dot: "bg-[var(--color-warning)]" },
  warning: { bg: "bg-[var(--color-warning-bg)]", text: "text-[var(--color-warning)]", dot: "bg-[var(--color-warning)]" },
  critical: { bg: "bg-[var(--color-danger-bg)]", text: "text-[var(--color-danger)]", dot: "bg-[var(--color-danger)]" },
};

export default function AnalyticsPage() {
  const [dashboard, setDashboard] = useState<CompleteAnalytics | null>(null);
  const [fsckStatus, setFsckStatus] = useState<FsckStatus | null>(null);
  const [fsckHistory, setFsckHistory] = useState<FsckCheckResult[]>([]);
  const [systemMetrics, setSystemMetrics] = useState<SystemMetrics | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [metricsAt, setMetricsAt] = useState<number | null>(null);
  const [fsckRunning, setFsckRunning] = useState(false);
  const { showToast } = useToast();

  const loadAll = useCallback(async () => {
    setRefreshing(true);
    const [analytics, system, fsck, history] = await Promise.allSettled([
      api.getDashboard(),
      api.getSystemMetrics(),
      api.getFsckStatus(),
      api.getFsckHistory(),
    ]);
    if (analytics.status === "fulfilled") setDashboard(analytics.value);
    if (system.status === "fulfilled") {
      setSystemMetrics(system.value);
      setMetricsAt(Date.now());
    }
    if (fsck.status === "fulfilled") {
      setFsckStatus(fsck.value);
      setFsckRunning(fsck.value.running);
    }
    if (history.status === "fulfilled") setFsckHistory(history.value.history || []);
    if (analytics.status === "rejected") {
      // Keep whatever was already on screen — a failed refresh on a flaky link
      // is no reason to throw away numbers the user is reading.
      showToast(
        analytics.reason instanceof Error ? analytics.reason.message : "Failed to load analytics",
        false,
      );
    }
    setLoading(false);
    setRefreshing(false);
  }, [showToast]);

  useEffect(() => { loadAll(); }, [loadAll]);

  useEffect(() => {
    const id = setInterval(async () => {
      // A backgrounded tab on a parked car shouldn't keep waking the Pi.
      if (document.hidden) return;
      try {
        setSystemMetrics(await api.getSystemMetrics());
        setMetricsAt(Date.now());
      } catch { /* transient — the next tick retries */ }
    }, METRICS_POLL_MS);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    if (!fsckRunning) return;
    const interval = setInterval(async () => {
      try {
        const status = await api.getFsckStatus();
        setFsckStatus(status);
        if (!status.running) {
          setFsckRunning(false);
          showToast("Filesystem check completed");
          const history = await api.getFsckHistory();
          setFsckHistory(history.history || []);
        }
      } catch { /* polling error */ }
    }, 3000);
    return () => clearInterval(interval);
  }, [fsckRunning, showToast]);

  const handleStartFsck = async (partitions: string[] | undefined, mode: FsckMode) => {
    // Repair writes non-interactive fixes to a filesystem holding evidence, and
    // its button sits next to the read-only Quick Check.
    if (
      mode === "repair" &&
      !confirm(
        `Run repair on ${partitions?.join(", ") ?? "all partitions"}? This writes fixes to the filesystem and can take several minutes.`,
      )
    ) {
      return;
    }
    try {
      await api.startFsck(partitions, mode);
      showToast(`${mode === "repair" ? "Repair" : "Check"} started`);
      setFsckRunning(true);
      setFsckStatus(await api.getFsckStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleCancelFsck = async () => {
    try {
      await api.cancelFsck();
      showToast("Check cancelled");
      setFsckRunning(false);
      setFsckStatus(await api.getFsckStatus());
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  // The storage walk can fail on its own while the fsck endpoints answer fine,
  // so a null dashboard degrades the page instead of replacing it.
  const health = dashboard?.storage_health;
  const style = HEALTH_STYLES[health?.status ?? ""] || HEALTH_STYLES.healthy;
  const alerts = health?.alerts ?? [];
  const recommendations = health?.recommendations ?? [];
  const partitionUsage = dashboard?.partition_usage ?? [];
  const videoStatistics = dashboard?.video_statistics ?? [];
  const folderBreakdown = dashboard?.folder_breakdown ?? [];

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Storage Analytics</h1>
        <button
          onClick={loadAll}
          disabled={refreshing}
          className="rounded-sm border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)] disabled:opacity-50"
        >
          {refreshing ? "Refreshing…" : "Refresh"}
        </button>
      </div>

      {!dashboard && (
        <section className="flex flex-col items-center justify-center gap-4 rounded bg-[var(--color-bg-card)] p-12 shadow-sm">
          <p className="text-[var(--color-danger)]">Could not load storage analytics.</p>
          <button
            onClick={loadAll}
            disabled={refreshing}
            className="rounded-sm bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {refreshing ? "Retrying…" : "Retry"}
          </button>
        </section>
      )}

      {/* Raspberry Pi System Stats */}
      {systemMetrics && (
        <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Raspberry Pi System</h2>
            {metricsAt && (
              <span className="text-xs text-[var(--color-text-muted)]">
                Updated {new Date(metricsAt).toLocaleTimeString()}
              </span>
            )}
          </div>
          <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <div className="rounded bg-[var(--color-bg-card-nested)] p-4">
              <p className="text-xs text-[var(--color-text-muted)]">CPU Usage</p>
              <p className="mt-1 text-2xl font-bold text-[var(--color-text-primary)]">
                {systemMetrics.cpu.usage_percent.toFixed(1)}%
              </p>
              <p className="mt-1 text-xs text-[var(--color-text-muted)]">{systemMetrics.cpu.cores} cores</p>
            </div>

            <div className="rounded bg-[var(--color-bg-card-nested)] p-4">
              <p className="text-xs text-[var(--color-text-muted)]">CPU Capacity</p>
              <p className="mt-1 text-2xl font-bold text-[var(--color-text-primary)]">
                {formatCpuCapacity(systemMetrics.cpu.capacity_mhz)}
              </p>
              <p className="mt-1 text-xs text-[var(--color-text-muted)]">
                Temp: {systemMetrics.cpu.temp_c ? `${systemMetrics.cpu.temp_c.toFixed(1)} °C` : "N/A"}
              </p>
            </div>

            {systemMetrics.power.watts !== undefined && (
              <div className="rounded bg-[var(--color-bg-card-nested)] p-4">
                <p className="text-xs text-[var(--color-text-muted)]">Power Draw</p>
                <p className="mt-1 text-2xl font-bold text-[var(--color-text-primary)]">
                  {systemMetrics.power.watts.toFixed(2)} W
                </p>
                <p className="mt-1 text-xs text-[var(--color-text-muted)]">Device sensor dependent</p>
              </div>
            )}

            <div className="rounded bg-[var(--color-bg-card-nested)] p-4">
              <p className="text-xs text-[var(--color-text-muted)]">RAM</p>
              <p className="mt-1 text-2xl font-bold text-[var(--color-text-primary)]">
                {formatBytes(systemMetrics.ram.used_bytes)} / {formatBytes(systemMetrics.ram.total_bytes)}
              </p>
              <div className="mt-2 h-3 overflow-hidden rounded-full bg-[var(--color-border)]">
                <div
                  className="h-full rounded-full bg-[var(--color-accent)] transition-all"
                  style={{
                    width: `${systemMetrics.ram.total_bytes > 0 ? (systemMetrics.ram.used_bytes / systemMetrics.ram.total_bytes) * 100 : 0}%`,
                  }}
                />
              </div>
            </div>
          </div>
        </section>
      )}

      {/* Health Card */}
      {dashboard && (
      <section className={`rounded p-6 shadow-sm ${style.bg}`}>
        <div className="flex items-center gap-3">
          <span className={`h-3 w-3 rounded-full ${style.dot}`} />
          <h2 className={`text-lg font-bold capitalize ${style.text}`}>{health?.status}</h2>
          <span className={`ml-auto text-sm ${style.text}`}>Score: {health?.score}/100</span>
        </div>
        {alerts.length > 0 && (
          <ul className="mt-3 space-y-1">
            {alerts.map((a, i) => (
              <li key={i} className={`text-xs ${style.text} opacity-80`}>&bull; {a}</li>
            ))}
          </ul>
        )}
        {recommendations.length > 0 && (
          <div className="mt-3">
            <p className={`text-xs font-semibold ${style.text}`}>Recommendations:</p>
            <ul className="mt-1 space-y-0.5">
              {recommendations.map((r, i) => (
                <li key={i} className={`text-xs ${style.text} opacity-70`}>&rarr; {r}</li>
              ))}
            </ul>
          </div>
        )}
      </section>
      )}

      {/* Recording Estimate */}
      {dashboard?.recording_estimate && Object.keys(dashboard.recording_estimate).length > 0 && (
        <div className="rounded bg-[var(--color-bg-card)] p-4 shadow-sm">
          <span className="text-sm text-[var(--color-text-muted)]">Estimated recording time remaining</span>
          <div className="mt-2 grid grid-cols-2 gap-4 sm:grid-cols-3">
            {Object.entries(dashboard.recording_estimate).map(([key, hours]) => (
              <div key={key}>
                <p className="text-xs text-[var(--color-text-muted)] capitalize">{key}</p>
                <p className="text-xl font-bold text-[var(--color-text-primary)]">{(hours as number).toFixed(1)}h</p>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Partition Usage */}
      {dashboard && (
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Partition Usage</h2>
        <div className="mt-4 space-y-4">
          {partitionUsage.map((p: PartitionUsage) => (
            <div key={p.name}>
              <div className="flex items-center justify-between text-sm">
                <span className="font-medium text-[var(--color-text-primary)]">{p.label || p.name}</span>
                <span className="text-[var(--color-text-muted)]">{formatBytes(p.used_bytes)} / {formatBytes(p.total_bytes)} <span className="ml-2 text-xs">({p.percent_used.toFixed(1)}%)</span></span>
              </div>
              <div className="mt-1.5 h-3 overflow-hidden rounded-full bg-[var(--color-border)]">
                <div className={`h-full rounded-full transition-all ${p.percent_used > 90 ? "bg-[var(--color-danger)]" : p.percent_used > 70 ? "bg-[var(--color-warning)]" : "bg-[var(--color-accent)]"}`} style={{ width: `${p.percent_used}%` }} />
              </div>
              <p className="mt-1 text-xs text-[var(--color-text-muted)]">{formatBytes(p.free_bytes)} free</p>
            </div>
          ))}
        </div>
      </section>
      )}

      {/* Video Stats */}
      {videoStatistics.length > 0 && (
        <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
          <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Video Statistics</h2>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-[var(--color-border)]">
                  <th className="pb-2 pr-4 font-medium text-[var(--color-text-muted)]">Folder</th>
                  <th className="pb-2 pr-4 text-right font-medium text-[var(--color-text-muted)]">Events</th>
                  <th className="pb-2 text-right font-medium text-[var(--color-text-muted)]">Size</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--color-border)]">
                {videoStatistics.map((vs: VideoStats) => (
                  <tr key={vs.folder}>
                    <td className="py-2 pr-4 font-medium text-[var(--color-text-primary)]">{vs.folder}</td>
                    <td className="py-2 pr-4 text-right tabular-nums text-[var(--color-text-secondary)]">{vs.count}</td>
                    <td className="py-2 text-right tabular-nums text-[var(--color-text-secondary)]">{formatBytes(vs.size_bytes)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {/* Folder Breakdown */}
      {folderBreakdown.length > 0 && (
        <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
          <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Folder Breakdown</h2>
          <div className="mt-4 space-y-2">
            {folderBreakdown.map((fb) => (
              <div key={fb.name}>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-[var(--color-text-primary)]">{fb.name}</span>
                  <span className="flex items-center gap-2 text-[var(--color-text-muted)]">
                    <span>{fb.count} items</span>
                    <span>{fb.size_mb.toFixed(1)} MB</span>
                    <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${fb.priority === "high" ? "bg-[var(--color-danger-bg)] text-[var(--color-danger)]" : fb.priority === "medium" ? "bg-[var(--color-warning-bg)] text-[var(--color-warning)]" : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"}`}>
                      {fb.priority}
                    </span>
                  </span>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Fsck Controls */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <div className="flex items-baseline justify-between gap-4">
          <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Filesystem Check</h2>
          <p className="text-xs text-[var(--color-text-muted)]">
            Quick = read-only verification · Repair = applies non-interactive fixes
          </p>
        </div>

        {fsckStatus?.running && (
          <div className="mt-4 rounded-sm bg-[var(--color-accent-subtle)] p-4">
            <div className="flex items-center justify-between">
              <p className="text-sm font-medium text-[var(--color-accent)]">
                Running on {fsckStatus.partition || "…"}
              </p>
              <button onClick={handleCancelFsck} className="rounded bg-[var(--color-danger-bg)] px-3 py-1.5 text-sm font-medium text-[var(--color-danger)] transition-colors hover:opacity-80">
                Cancel
              </button>
            </div>
          </div>
        )}

        {/* Per-partition controls */}
        <div className="mt-4 space-y-2">
          {partitionUsage.map((p) => (
            <div key={p.name} className="flex flex-wrap items-center justify-between gap-3 rounded-sm bg-[var(--color-bg-card-nested)] px-3 py-2">
              <div className="text-sm">
                <span className="font-medium text-[var(--color-text-primary)]">{p.label || p.name}</span>
                <span className="ml-2 text-xs text-[var(--color-text-muted)]">{p.name}</span>
              </div>
              <div className="flex gap-2">
                <button
                  onClick={() => handleStartFsck([p.name], "quick")}
                  disabled={fsckRunning}
                  className="rounded bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-white shadow-sm transition-colors hover:bg-[var(--color-accent-hover)] disabled:opacity-50"
                >
                  Quick Check
                </button>
                <button
                  onClick={() => handleStartFsck([p.name], "repair")}
                  disabled={fsckRunning}
                  className="rounded-sm border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)] disabled:opacity-50"
                >
                  Repair
                </button>
              </div>
            </div>
          ))}
        </div>

        {/* All-partitions controls */}
        <div className="mt-4 flex flex-wrap gap-3 border-t border-[var(--color-border)] pt-4">
          <button onClick={() => handleStartFsck(undefined, "quick")} disabled={fsckRunning} className="rounded bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:bg-[var(--color-accent-hover)] disabled:opacity-50">
            Quick Check (all)
          </button>
          <button onClick={() => handleStartFsck(undefined, "repair")} disabled={fsckRunning} className="rounded-sm border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)] disabled:opacity-50">
            Repair (all)
          </button>
        </div>

        {fsckHistory.length > 0 && (
          <div className="mt-6">
            <h3 className="text-sm font-semibold text-[var(--color-text-secondary)]">History</h3>
            <div className="mt-2 space-y-2">
              {fsckHistory.slice().reverse().slice(0, 10).map((h, i) => {
                const detail = (h.output || "").trim() || h.error || "";
                return (
                  <details key={i} className="group rounded-sm bg-[var(--color-bg-card-nested)] px-3 py-2 text-sm">
                    <summary className="flex cursor-pointer list-none items-center justify-between gap-3">
                      <div className="flex flex-wrap items-baseline gap-2">
                        <span className="font-medium text-[var(--color-text-primary)]">{h.partition}</span>
                        {h.fs_type && (
                          <span className="rounded bg-[var(--color-bg-tertiary)] px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-[var(--color-text-muted)]">
                            {h.fs_type}
                          </span>
                        )}
                        {h.mode && (
                          <span className="rounded bg-[var(--color-bg-tertiary)] px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-[var(--color-text-muted)]">
                            {h.mode}
                          </span>
                        )}
                        <span className="text-xs text-[var(--color-text-muted)]">{formatDate(h.started_at)}</span>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className="text-[10px] tabular-nums text-[var(--color-text-muted)]">
                          exit {h.exit_code}
                        </span>
                        <span className={`rounded px-2 py-0.5 text-xs font-medium ${h.status === "done" ? "bg-[var(--color-success-bg)] text-[var(--color-success)]" : h.status === "failed" ? "bg-[var(--color-danger-bg)] text-[var(--color-danger)]" : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]"}`}>
                          {h.status}
                        </span>
                        <span className="text-xs text-[var(--color-text-muted)] transition-transform group-open:rotate-180">▾</span>
                      </div>
                    </summary>
                    {detail ? (
                      <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-black/30 p-2 text-[11px] leading-snug text-[var(--color-text-secondary)]">
                        {detail}
                      </pre>
                    ) : (
                      <p className="mt-2 text-xs italic text-[var(--color-text-muted)]">No output captured.</p>
                    )}
                  </details>
                );
              })}
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
