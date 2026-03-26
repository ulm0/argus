"use client";

import { useEffect, useState, useCallback } from "react";
import * as api from "@/lib/api";
import type { CleanupPolicy, CleanupReport } from "@/lib/types";

export default function CleanupPage() {
  const [policies, setPolicies] = useState<Record<string, CleanupPolicy>>({});
  const [preview, setPreview] = useState<CleanupReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [executing, setExecuting] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  const showToast = useCallback((msg: string, ok = true) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  }, []);

  useEffect(() => {
    api
      .getSettings()
      .then((res) => setPolicies(res.policies || {}))
      .catch(() => showToast("Failed to load cleanup settings", false))
      .finally(() => setLoading(false));
  }, [showToast]);

  const updatePolicy = (folder: string, updates: Partial<CleanupPolicy>) => {
    setPolicies((prev) => ({
      ...prev,
      [folder]: { ...prev[folder], ...updates },
    }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await api.saveSettings(policies);
      showToast("Settings saved");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Save failed", false);
    } finally {
      setSaving(false);
    }
  };

  const handlePreview = async () => {
    setPreviewing(true);
    setPreview(null);
    try {
      const res = await api.getPreview();
      setPreview(res.report);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Preview failed", false);
    } finally {
      setPreviewing(false);
    }
  };

  const handleExecute = async (dryRun: boolean) => {
    if (!dryRun && !confirm("Execute cleanup? This will permanently delete files.")) return;
    setExecuting(true);
    try {
      const report = await api.executeCleanup(dryRun);
      setPreview(report);
      showToast(dryRun ? "Dry run complete" : `Cleanup done: ${report.deleted_count} files deleted`);
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

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      {toast && (
        <div className={`fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-sm px-4 py-3 text-sm font-medium shadow-lg ${toast.ok ? "bg-[var(--color-success)] text-white" : "bg-[var(--color-danger)] text-white"}`}>
          {toast.msg}
        </div>
      )}

      <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Cleanup Settings</h1>

      {/* Folder Policies */}
      <section className="space-y-4">
        {Object.entries(policies).map(([folder, policy]) => (
          <div key={folder} className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">{folder}</h2>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={policy.enabled} onChange={(e) => updatePolicy(folder, { enabled: e.target.checked })} className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]" />
                <span className="text-sm text-[var(--color-text-secondary)]">Enabled</span>
              </label>
            </div>

            {policy.enabled && (
              <div className="mt-4 space-y-4">
                <label className="flex items-center gap-2 text-sm text-[var(--color-text-secondary)]">
                  <input type="checkbox" checked={policy.boot_cleanup} onChange={(e) => updatePolicy(folder, { boot_cleanup: e.target.checked })} className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]" />
                  Run on boot
                </label>

                {/* Age policy */}
                <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-4">
                  <label className="flex items-center gap-2 text-sm font-medium text-[var(--color-text-secondary)]">
                    <input type="checkbox" checked={policy.age_based?.enabled ?? false} onChange={(e) => updatePolicy(folder, { age_based: { ...(policy.age_based ?? { enabled: false, max_days: 30 }), enabled: e.target.checked } })} className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]" />
                    Age-based cleanup
                  </label>
                  {policy.age_based?.enabled && (
                    <div className="mt-2">
                      <label className="block text-xs text-[var(--color-text-muted)]">Max age (days)</label>
                      <input type="number" min={1} value={policy.age_based.max_days} onChange={(e) => updatePolicy(folder, { age_based: { ...policy.age_based!, max_days: Number(e.target.value) } })} className="mt-1 block w-32 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-1.5 text-sm text-[var(--color-text-primary)]" />
                    </div>
                  )}
                </div>

                {/* Size policy */}
                <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-4">
                  <label className="flex items-center gap-2 text-sm font-medium text-[var(--color-text-secondary)]">
                    <input type="checkbox" checked={policy.size_based?.enabled ?? false} onChange={(e) => updatePolicy(folder, { size_based: { ...(policy.size_based ?? { enabled: false, max_gb: 10 }), enabled: e.target.checked } })} className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]" />
                    Size-based cleanup
                  </label>
                  {policy.size_based?.enabled && (
                    <div className="mt-2">
                      <label className="block text-xs text-[var(--color-text-muted)]">Max size (GB)</label>
                      <input type="number" min={0.1} step={0.1} value={policy.size_based.max_gb} onChange={(e) => updatePolicy(folder, { size_based: { ...policy.size_based!, max_gb: Number(e.target.value) } })} className="mt-1 block w-32 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-1.5 text-sm text-[var(--color-text-primary)]" />
                    </div>
                  )}
                </div>

                {/* Count policy */}
                <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-4">
                  <label className="flex items-center gap-2 text-sm font-medium text-[var(--color-text-secondary)]">
                    <input type="checkbox" checked={policy.count_based?.enabled ?? false} onChange={(e) => updatePolicy(folder, { count_based: { ...(policy.count_based ?? { enabled: false, max_count: 100 }), enabled: e.target.checked } })} className="h-4 w-4 rounded border-[var(--color-border)] text-[var(--color-accent)] focus:ring-[var(--color-accent)]" />
                    Count-based cleanup
                  </label>
                  {policy.count_based?.enabled && (
                    <div className="mt-2">
                      <label className="block text-xs text-[var(--color-text-muted)]">Max count</label>
                      <input type="number" min={1} value={policy.count_based.max_count} onChange={(e) => updatePolicy(folder, { count_based: { ...policy.count_based!, max_count: Number(e.target.value) } })} className="mt-1 block w-32 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-1.5 text-sm text-[var(--color-text-primary)]" />
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        ))}

        {Object.keys(policies).length === 0 && (
          <div className="rounded bg-[var(--color-bg-card)] p-12 text-center shadow-sm">
            <p className="text-sm text-[var(--color-text-muted)]">
              No folder policies configured. The system will detect folders automatically when the partition is mounted.
            </p>
          </div>
        )}
      </section>

      {/* Actions */}
      <div className="flex flex-wrap gap-3">
        <button onClick={handleSave} disabled={saving} className="rounded-sm bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:bg-[var(--color-accent-hover)] disabled:opacity-50">
          {saving ? "Saving..." : "Save Settings"}
        </button>
        <button onClick={handlePreview} disabled={previewing} className="rounded-sm border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)] disabled:opacity-50">
          {previewing ? "Calculating..." : "Preview Impact"}
        </button>
        <button onClick={() => handleExecute(true)} disabled={executing} className="rounded-sm border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)] disabled:opacity-50">
          Dry Run
        </button>
        <button onClick={() => handleExecute(false)} disabled={executing} className="rounded-sm bg-[var(--color-danger)] px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:opacity-90 disabled:opacity-50">
          {executing ? "Running..." : "Execute Cleanup"}
        </button>
      </div>

      {/* Preview / Report */}
      {preview && (
        <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">
              {preview.dry_run ? "Dry Run Report" : "Cleanup Report"}
            </h2>
            {preview.dry_run && (
              <span className="rounded bg-[var(--color-warning-bg)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-warning)]">Dry Run</span>
            )}
          </div>
          <div className="mt-4 grid grid-cols-2 gap-4">
            <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-3">
              <span className="text-xs text-[var(--color-text-muted)]">Files deleted</span>
              <p className="text-xl font-bold text-[var(--color-text-primary)]">{preview.deleted_count}</p>
            </div>
            <div className="rounded-sm bg-[var(--color-bg-card-nested)] p-3">
              <span className="text-xs text-[var(--color-text-muted)]">Space freed</span>
              <p className="text-xl font-bold text-[var(--color-text-primary)]">{preview.deleted_size_gb.toFixed(2)} GB</p>
            </div>
          </div>
          {preview.errors && preview.errors.length > 0 && (
            <div className="mt-4">
              <p className="text-xs font-semibold text-[var(--color-danger)]">Errors:</p>
              <ul className="mt-1 space-y-0.5">
                {preview.errors.map((err, i) => (
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
