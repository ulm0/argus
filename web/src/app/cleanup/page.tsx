"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import * as api from "@/lib/api";
import type { CleanupPlan, CleanupPolicy, CleanupReport } from "@/lib/types";
import { formatBytes } from "@/lib/format";
import { useToast } from "@/components/Toast";

const DEFAULT_KEEP_LAST = 50;

type DraftPolicies = Record<string, CleanupPolicy>;

export default function CleanupPage() {
  const [serverPolicies, setServerPolicies] = useState<DraftPolicies>({});
  const [draft, setDraft] = useState<DraftPolicies>({});
  const [plan, setPlan] = useState<CleanupPlan | null>(null);
  const [lastReport, setLastReport] = useState<CleanupReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [executing, setExecuting] = useState(false);
  const { showToast } = useToast();

  const reload = useCallback(() => {
    setLoading(true);
    api
      .getSettings()
      .then((res) => {
        const policies = res.policies || {};
        setServerPolicies(policies);
        setDraft(policies);
      })
      .catch(() => showToast("Failed to load cleanup settings", false))
      .finally(() => setLoading(false));
  }, [showToast]);

  useEffect(() => {
    reload();
  }, [reload]);

  const dirty = useMemo(
    () => JSON.stringify(draft) !== JSON.stringify(serverPolicies),
    [draft, serverPolicies],
  );

  const updatePolicy = (folder: string, updates: Partial<CleanupPolicy>) => {
    setDraft((prev) => ({
      ...prev,
      [folder]: { ...prev[folder], ...updates },
    }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      // Clamp invalid keep_last so the server doesn't see 0/NaN.
      const sanitized: DraftPolicies = {};
      for (const [folder, p] of Object.entries(draft)) {
        const keep = Number.isFinite(p.keep_last) && p.keep_last > 0 ? p.keep_last : DEFAULT_KEEP_LAST;
        sanitized[folder] = { ...p, keep_last: keep };
      }
      await api.saveSettings(sanitized);
      setServerPolicies(sanitized);
      setDraft(sanitized);
      setPlan(null);
      showToast("Settings saved");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Save failed", false);
    } finally {
      setSaving(false);
    }
  };

  const handlePreview = async () => {
    setPreviewing(true);
    setLastReport(null);
    try {
      const res = await api.getPreview();
      setPlan(res.plan);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Preview failed", false);
    } finally {
      setPreviewing(false);
    }
  };

  const handleExecute = async () => {
    setExecuting(true);
    try {
      // The server recalculates the plan at execute time, so confirm against a
      // plan fetched now — not whatever the Impact card happened to show.
      const { plan: fresh } = await api.getPreview();
      setPlan(fresh);
      if (
        !confirm(
          `Delete ${fresh.total_count} events (${formatBytes(fresh.total_size)})? This cannot be undone.`,
        )
      ) {
        return;
      }
      const report = await api.executeCleanup(false);
      setLastReport(report);
      setPlan(null);
      showToast(`Cleanup done: ${report.deleted_count} event${report.deleted_count === 1 ? "" : "s"} deleted`);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Execution failed", false);
    } finally {
      setExecuting(false);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  const folders = Object.keys(draft).sort();

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      <div>
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Cleanup</h1>
        <p className="mt-1 text-sm text-[var(--color-text-muted)]">
          For each folder, keep the most recent N events. Older events are deleted whole (all cameras + metadata together).
        </p>
      </div>

      <section className="space-y-4">
        {folders.map((folder) => {
          const policy = draft[folder];
          return (
            <div key={folder} className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
              <div className="flex items-center justify-between gap-4">
                <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">{folder}</h2>
                <label className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={policy.enabled}
                    onChange={(e) => updatePolicy(folder, { enabled: e.target.checked })}
                    className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]"
                  />
                  <span className="text-sm text-[var(--color-text-secondary)]">Enabled</span>
                </label>
              </div>

              {policy.enabled && (
                <div className="mt-4 grid gap-4 sm:grid-cols-2">
                  <div>
                    <label className="block text-xs text-[var(--color-text-muted)]">
                      Keep last N events
                    </label>
                    <input
                      type="number"
                      min={1}
                      value={policy.keep_last || ""}
                      onChange={(e) =>
                        updatePolicy(folder, { keep_last: Number(e.target.value) })
                      }
                      className="mt-1 block w-32 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-1.5 text-sm text-[var(--color-text-primary)]"
                    />
                  </div>

                  <label className="mt-1 flex items-center gap-2 text-sm text-[var(--color-text-secondary)] sm:mt-6">
                    <input
                      type="checkbox"
                      checked={policy.boot_cleanup}
                      onChange={(e) => updatePolicy(folder, { boot_cleanup: e.target.checked })}
                      className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]"
                    />
                    Also run automatically on boot
                  </label>
                </div>
              )}
            </div>
          );
        })}

        {folders.length === 0 && (
          <div className="rounded bg-[var(--color-bg-card)] p-12 text-center shadow-sm">
            <p className="text-sm text-[var(--color-text-muted)]">
              No folders detected. Make sure part1 is mounted (Edit mode), then reload.
            </p>
          </div>
        )}
      </section>

      <div className="flex flex-wrap gap-3">
        <button
          onClick={handleSave}
          disabled={saving || !dirty}
          className="rounded-sm bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:bg-[var(--color-accent-hover)] disabled:opacity-50"
        >
          {saving ? "Saving..." : dirty ? "Save changes" : "Saved"}
        </button>
        <button
          onClick={handlePreview}
          disabled={previewing || dirty}
          title={dirty ? "Save changes first" : "Calculate which events would be deleted"}
          className="rounded-sm border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)] disabled:opacity-50"
        >
          {previewing ? "Calculating..." : "Calculate impact"}
        </button>
        <button
          onClick={handleExecute}
          disabled={executing || dirty || folders.length === 0}
          title={dirty ? "Save changes first" : "Run cleanup now"}
          className="rounded-sm bg-[var(--color-danger)] px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50"
        >
          {executing ? "Running..." : "Clean now"}
        </button>
      </div>

      {plan && (
        <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Impact</h2>
            <span className="rounded bg-[var(--color-warning-bg)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-warning)]">
              Preview
            </span>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-4">
            <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-3">
              <span className="text-xs text-[var(--color-text-muted)]">Events to delete</span>
              <p className="text-xl font-bold text-[var(--color-text-primary)]">{plan.total_count}</p>
            </div>
            <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-3">
              <span className="text-xs text-[var(--color-text-muted)]">Space to free</span>
              <p className="text-xl font-bold text-[var(--color-text-primary)]">{formatBytes(plan.total_size)}</p>
            </div>
          </div>

          {Object.entries(plan.breakdown_by_folder).length > 0 && (
            <div className="mt-4 space-y-3">
              {Object.entries(plan.breakdown_by_folder).map(([folder, events]) => (
                <div key={folder} className="rounded-sm bg-[var(--color-bg-card-nested)] p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-semibold text-[var(--color-text-primary)]">{folder}</span>
                    <span className="text-xs text-[var(--color-text-muted)]">
                      {events.length} event{events.length === 1 ? "" : "s"}
                    </span>
                  </div>
                  {events.length > 0 && (
                    <p className="mt-1 truncate text-xs text-[var(--color-text-muted)]" title={events.map((e) => e.name).join(", ")}>
                      Oldest: {events[events.length - 1].name} … Newest in deletion: {events[0].name}
                    </p>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      {lastReport && (
        <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
          <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Last cleanup</h2>
          <div className="mt-4 grid grid-cols-2 gap-4">
            <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-3">
              <span className="text-xs text-[var(--color-text-muted)]">Events deleted</span>
              <p className="text-xl font-bold text-[var(--color-text-primary)]">{lastReport.deleted_count}</p>
            </div>
            <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-3">
              <span className="text-xs text-[var(--color-text-muted)]">Space freed</span>
              <p className="text-xl font-bold text-[var(--color-text-primary)]">
                {formatBytes(lastReport.deleted_size_gb * 1024 * 1024 * 1024)}
              </p>
            </div>
          </div>
          {lastReport.errors && lastReport.errors.length > 0 && (
            <div className="mt-4">
              <p className="text-xs font-semibold text-[var(--color-danger)]">Errors:</p>
              <ul className="mt-1 space-y-0.5">
                {lastReport.errors.map((err, i) => (
                  <li key={i} className="text-xs text-[var(--color-danger)]">{err}</li>
                ))}
              </ul>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
