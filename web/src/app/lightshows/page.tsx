"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import * as api from "@/lib/api";
import type { LightShow } from "@/lib/types";
import { useFeatureGuard } from "@/hooks/useFeatureGuard";
import { useToast } from "@/components/Toast";
import EditModeBanner from "@/components/EditModeBanner";

// The API returns on-disk partition keys; users know the partitions by purpose.
const PARTITION_LABELS: Record<string, string> = {
  part1: "Dashcam",
  part2: "Light Show",
  part3: "Music",
};

export default function LightshowsPage() {
  const { available, loading: featureLoading } = useFeatureGuard("shows_available");
  const { showToast } = useToast();
  const [shows, setShows] = useState<LightShow[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [uploadDone, setUploadDone] = useState(0);
  const [uploadTotal, setUploadTotal] = useState(0);
  const [uploadResults, setUploadResults] = useState<{ name: string; error?: string }[]>([]);
  const [dragOver, setDragOver] = useState(false);
  const [playing, setPlaying] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  const audioRef = useRef<HTMLAudioElement | null>(null);

  const loadShows = useCallback(async () => {
    try {
      const res = await api.getShows();
      setShows(res.shows || []);
    } catch {
      showToast("Failed to load light shows", false);
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    loadShows();
  }, [loadShows]);

  // Stop any in-progress playback when the page unmounts, otherwise the audio
  // keeps playing after the user navigates away with no UI left to stop it.
  useEffect(
    () => () => {
      audioRef.current?.pause();
      audioRef.current = null;
    },
    [],
  );

  // Per-file try/catch: one rejected fseq must not abandon the rest of the batch,
  // and the user needs to know which file the device refused.
  const handleUpload = async (fileList: FileList | File[]) => {
    const files = Array.from(fileList);
    if (!files.length || uploading) return;
    setUploading(true);
    setUploadResults([]);
    setUploadDone(0);
    setUploadTotal(files.length);
    const results: { name: string; error?: string }[] = [];
    for (const file of files) {
      try {
        await api.uploadShow(file);
        results.push({ name: file.name });
      } catch (e) {
        results.push({ name: file.name, error: e instanceof Error ? e.message : "Upload failed" });
      }
      setUploadResults([...results]);
      setUploadDone(results.length);
    }
    setUploading(false);
    const failed = results.filter((r) => r.error).length;
    showToast(
      failed
        ? `${results.length - failed} uploaded, ${failed} failed`
        : `Uploaded ${results.length} file(s)`,
      failed === 0,
    );
    await loadShows();
  };

  const handleDelete = async (partition: string, baseName: string) => {
    if (!confirm(`Delete "${baseName}" and all associated files?`)) return;
    try {
      await api.deleteShow(partition, baseName);
      showToast("Light show deleted");
      await loadShows();
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Delete failed", false);
    }
  };

  const handlePlayAudio = (partition: string, filename: string) => {
    const key = `${partition}/${filename}`;
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current = null;
    }
    if (playing === key) {
      setPlaying(null);
      return;
    }
    const audio = new Audio(api.playShowURL(partition, filename));
    audio.onended = () => setPlaying(null);
    // A missing or unplayable file otherwise leaves the pause icon stuck forever.
    audio.onerror = () => setPlaying(null);
    audio.play().catch(() => {
      setPlaying(null);
      showToast("Playback failed", false);
    });
    audioRef.current = audio;
    setPlaying(key);
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
        <h1 className="text-xl font-bold text-[var(--color-text-primary)]">Light Shows Disabled</h1>
        <p className="max-w-sm text-sm text-[var(--color-text-muted)]">
          Light shows are not enabled. Set <code className="rounded bg-[var(--color-bg-tertiary)] px-1">lightshow_enabled: true</code> (and <code className="rounded bg-[var(--color-bg-tertiary)] px-1">part2_enabled: true</code>) in your <code className="rounded bg-[var(--color-bg-tertiary)] px-1">config.yaml</code> and restart Argus.
        </p>
      </div>
    );
  }

  const q = query.trim().toLowerCase();
  const visibleShows = q
    ? shows.filter((s) => s.base_name.toLowerCase().includes(q))
    : shows;

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Light Shows</h1>

      <EditModeBanner />

      {/* Upload Zone */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Upload</h2>
        <label
          onDragOver={(e) => { e.preventDefault(); if (!uploading) setDragOver(true); }}
          onDragLeave={() => setDragOver(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragOver(false);
            if (!uploading) handleUpload(e.dataTransfer.files);
          }}
          className={`mt-4 flex cursor-pointer flex-col items-center rounded-sm border-2 border-dashed p-8 transition-colors ${dragOver ? "border-[var(--color-accent)] bg-[var(--color-accent-subtle)]" : "border-[var(--color-border)] hover:border-[var(--color-text-muted)]"} ${uploading ? "pointer-events-none opacity-60" : ""}`}
        >
          <svg className="h-8 w-8 text-[var(--color-text-muted)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
          </svg>
          <p className="mt-2 text-sm font-medium text-[var(--color-text-secondary)]">
            Drop files here or click to browse
          </p>
          <p className="text-xs text-[var(--color-text-muted)]">.fseq, .mp3, .wav, .zip</p>
          <input
            type="file"
            multiple
            accept=".fseq,.mp3,.wav,.zip"
            className="sr-only"
            onChange={(e) => {
              if (e.target.files) handleUpload(e.target.files);
              // Without this, re-picking the same file fires no change event.
              e.target.value = "";
            }}
          />
        </label>
        {(uploading || uploadResults.length > 0) && (
          <div className="mt-3 space-y-1">
            {uploading && (
              <p className="text-sm text-[var(--color-text-secondary)]">
                Uploading {Math.min(uploadDone + 1, uploadTotal)}/{uploadTotal}
              </p>
            )}
            {uploadResults.map((r) => (
              <p
                key={r.name}
                className={`truncate text-xs ${r.error ? "text-[var(--color-danger)]" : "text-[var(--color-success)]"}`}
              >
                {r.name} — {r.error ?? "uploaded"}
              </p>
            ))}
          </div>
        )}
      </section>

      {/* Show List */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Shows ({shows.length})</h2>
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Filter shows"
          className="mt-3 block w-full rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-2 text-sm text-[var(--color-text-primary)]"
        />
        {visibleShows.length === 0 ? (
          <p className="mt-4 text-sm text-[var(--color-text-muted)]">
            {shows.length === 0 ? "No light shows found." : "No shows match that filter."}
          </p>
        ) : (
          <div className="mt-4 divide-y divide-[var(--color-border)]">
            {visibleShows.map((s) => {
              const audioKey = s.audio_file ? `${s.partition_key}/${s.audio_file}` : null;
              return (
                <div key={`${s.partition_key}-${s.base_name}`} className="py-4">
                  <div className="flex items-start justify-between">
                    <div>
                      <h3 className="font-semibold text-[var(--color-text-primary)]">{s.base_name}</h3>
                      <div className="mt-1 flex flex-wrap items-center gap-2">
                        {s.fseq_file && (
                          <span className="rounded bg-[var(--color-accent-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-accent-text)]">FSEQ</span>
                        )}
                        {s.audio_file && (
                          <span className="rounded bg-[var(--color-accent-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-accent)]">Audio</span>
                        )}
                        <span className="text-xs text-[var(--color-text-muted)]">
                          {PARTITION_LABELS[s.partition_key] ?? s.partition_key}
                        </span>
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      {s.audio_file && (
                        <button
                          onClick={() => handlePlayAudio(s.partition_key, s.audio_file!)}
                          aria-label={playing === audioKey ? `Stop ${s.base_name}` : `Play ${s.base_name}`}
                          title={playing === audioKey ? `Stop ${s.base_name}` : `Play ${s.base_name}`}
                          className={`rounded-sm p-2.5 transition-colors ${playing === audioKey ? "bg-[var(--color-accent-subtle)] text-[var(--color-accent)]" : "text-[var(--color-text-muted)] hover:bg-[var(--color-bg-tertiary)]"}`}
                        >
                          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            {playing === audioKey ? (
                              <path strokeLinecap="round" strokeLinejoin="round" d="M6 4h4v16H6zM14 4h4v16h-4z" />
                            ) : (
                              <path strokeLinecap="round" strokeLinejoin="round" d="M5 3l14 9-14 9V3z" />
                            )}
                          </svg>
                        </button>
                      )}
                      <a
                        href={api.downloadShowURL(s.partition_key, s.base_name)}
                        aria-label={`Download ${s.base_name}`}
                        title={`Download ${s.base_name}`}
                        className="rounded-sm p-2.5 text-[var(--color-text-muted)] transition-colors hover:bg-[var(--color-bg-tertiary)]"
                      >
                        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                        </svg>
                      </a>
                      <button
                        onClick={() => handleDelete(s.partition_key, s.base_name)}
                        aria-label={`Delete ${s.base_name}`}
                        title={`Delete ${s.base_name}`}
                        className="rounded-sm p-2.5 text-[var(--color-danger)] transition-colors hover:bg-[var(--color-danger-bg)]"
                      >
                        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                        </svg>
                      </button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}
