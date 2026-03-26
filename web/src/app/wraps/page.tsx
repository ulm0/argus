"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import * as api from "@/lib/api";
import type { WrapFile } from "@/lib/types";
import { useFeatureGuard } from "@/hooks/useFeatureGuard";

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

export default function WrapsPage() {
  const { available, loading: featureLoading } = useFeatureGuard("wraps_available");
  const [wraps, setWraps] = useState<WrapFile[]>([]);
  const [maxCount, setMaxCount] = useState(10);
  const [maxSize, setMaxSize] = useState(1024 * 1024);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);

  const showToast = useCallback((msg: string, ok = true) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadWraps = useCallback(async () => {
    try {
      const res = await api.getWraps();
      setWraps(res.wraps || []);
      setMaxCount(res.max_count);
      setMaxSize(res.max_size);
    } catch {
      showToast("Failed to load wraps", false);
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    loadWraps();
  }, [loadWraps]);

  const handleUpload = async (files: FileList | File[]) => {
    const fileArray = Array.from(files);
    const pngFiles = fileArray.filter((f) => f.name.toLowerCase().endsWith(".png"));

    if (pngFiles.length === 0) {
      showToast("Only PNG files are allowed", false);
      return;
    }

    const tooLarge = pngFiles.filter((f) => f.size > maxSize);
    if (tooLarge.length) {
      showToast(`${tooLarge.length} file(s) exceed ${formatBytes(maxSize)} limit`, false);
      return;
    }

    if (wraps.length + pngFiles.length > maxCount) {
      showToast(`Maximum ${maxCount} wraps allowed`, false);
      return;
    }

    setUploading(true);
    try {
      for (const file of pngFiles) {
        await api.uploadWrap(file);
      }
      showToast(`Uploaded ${pngFiles.length} wrap(s)`);
      await loadWraps();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Upload failed", false);
    } finally {
      setUploading(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    handleUpload(e.dataTransfer.files);
  };

  const handleDelete = async (partition: string, filename: string) => {
    if (!confirm(`Delete "${filename}"?`)) return;
    try {
      await api.deleteWrap(partition, filename);
      showToast("Wrap deleted");
      await loadWraps();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Delete failed", false);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  if (!featureLoading && !available) {
    return (
      <div className="flex min-h-full flex-col items-center justify-center gap-4 p-8 text-center">
        <svg className="h-12 w-12 text-[var(--color-text-muted)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
        </svg>
        <h1 className="text-xl font-bold text-[var(--color-text-primary)]">Custom Wraps Disabled</h1>
        <p className="max-w-sm text-sm text-[var(--color-text-muted)]">
          Wraps are not enabled. Set <code className="rounded bg-[var(--color-bg-tertiary)] px-1">wraps_enabled: true</code> (and <code className="rounded bg-[var(--color-bg-tertiary)] px-1">part2_enabled: true</code>) in your <code className="rounded bg-[var(--color-bg-tertiary)] px-1">config.yaml</code> and restart Argus.
        </p>
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

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Custom Wraps</h1>
        <span className="rounded bg-[var(--color-bg-tertiary)] px-3 py-1 text-sm font-medium text-[var(--color-text-secondary)]">
          {wraps.length} / {maxCount}
        </span>
      </div>

      {/* Upload Zone */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Upload</h2>
        <p className="mt-1 text-sm text-[var(--color-text-muted)]">
          PNG only &middot; 512&ndash;1024px &middot; Max {formatBytes(maxSize)} per file &middot; Up to {maxCount} total
        </p>
        <div
          onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
          onDragLeave={() => setDragOver(false)}
          onDrop={handleDrop}
          className={`mt-4 flex cursor-pointer flex-col items-center rounded-sm border-2 border-dashed p-8 transition-colors ${dragOver ? "border-[var(--color-accent)] bg-[var(--color-accent-subtle)]" : "border-[var(--color-border)] hover:border-[var(--color-text-muted)]"}`}
          onClick={() => fileInputRef.current?.click()}
        >
          <svg className="h-8 w-8 text-[var(--color-text-muted)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
          </svg>
          <p className="mt-2 text-sm font-medium text-[var(--color-text-secondary)]">
            {uploading ? "Uploading..." : "Drop PNG files here or click to browse"}
          </p>
          <input ref={fileInputRef} type="file" multiple accept=".png" className="hidden" onChange={(e) => e.target.files && handleUpload(e.target.files)} />
        </div>
      </section>

      {/* Thumbnail Grid */}
      {wraps.length === 0 ? (
        <div className="py-12 text-center text-sm text-[var(--color-text-muted)]">No custom wraps uploaded yet.</div>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          {wraps.map((w) => (
            <div key={`${w.partition_key}-${w.filename}`} className="group relative overflow-hidden rounded bg-[var(--color-bg-card)] shadow-sm transition-all hover:shadow-md">
              <div className="aspect-square bg-[var(--color-bg-tertiary)]">
                <img src={api.wrapThumbnailURL(w.partition_key, w.filename)} alt={w.filename} className="h-full w-full object-contain p-2" loading="lazy" />
              </div>
              <div className="p-3">
                <p className="truncate text-xs font-medium text-[var(--color-text-primary)]">{w.filename}</p>
                <p className="text-xs text-[var(--color-text-muted)]">{w.size_str} &middot; {w.width}&times;{w.height}</p>
              </div>
              <div className="absolute right-1.5 top-1.5 flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                <a href={api.downloadWrapURL(w.partition_key, w.filename)} className="rounded-sm bg-black/60 p-1.5 text-white backdrop-blur-sm transition-colors hover:bg-black/80" title="Download">
                  <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                  </svg>
                </a>
                <button onClick={() => handleDelete(w.partition_key, w.filename)} className="rounded-sm bg-[var(--color-danger)]/80 p-1.5 text-white backdrop-blur-sm transition-colors hover:bg-[var(--color-danger)]" title="Delete">
                  <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
