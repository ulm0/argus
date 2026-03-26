"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import * as api from "@/lib/api";
import type { LightShow } from "@/lib/types";
import { useFeatureGuard } from "@/hooks/useFeatureGuard";

export default function LightshowsPage() {
  const { available, loading: featureLoading } = useFeatureGuard("shows_available");
  const [shows, setShows] = useState<LightShow[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [playing, setPlaying] = useState<string | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);

  const audioRef = useRef<HTMLAudioElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const showToast = useCallback((msg: string, ok = true) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  }, []);

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

  const handleUpload = async (files: FileList | File[]) => {
    if (!files.length) return;
    setUploading(true);
    try {
      for (const file of Array.from(files)) {
        await api.uploadShow(file);
      }
      showToast(`Uploaded ${files.length} file(s)`);
      await loadShows();
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
    audio.play();
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

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      {toast && (
        <div className={`fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-sm px-4 py-3 text-sm font-medium shadow-lg ${toast.ok ? "bg-[var(--color-success)] text-white" : "bg-[var(--color-danger)] text-white"}`}>
          {toast.msg}
        </div>
      )}

      <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Light Shows</h1>

      {/* Upload Zone */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Upload</h2>
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
            {uploading ? "Uploading..." : "Drop files here or click to browse"}
          </p>
          <p className="text-xs text-[var(--color-text-muted)]">.fseq, .mp3, .wav, .zip</p>
          <input ref={fileInputRef} type="file" multiple accept=".fseq,.mp3,.wav,.zip" className="hidden" onChange={(e) => e.target.files && handleUpload(e.target.files)} />
        </div>
      </section>

      {/* Show List */}
      <section className="rounded bg-[var(--color-bg-card)] p-6 shadow-sm">
        <h2 className="text-lg font-semibold text-[var(--color-text-primary)]">Shows ({shows.length})</h2>
        {shows.length === 0 ? (
          <p className="mt-4 text-sm text-[var(--color-text-muted)]">No light shows found.</p>
        ) : (
          <div className="mt-4 divide-y divide-[var(--color-border)]">
            {shows.map((s) => {
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
                        <span className="text-xs text-[var(--color-text-muted)]">{s.partition_key}</span>
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      {s.audio_file && (
                        <button
                          onClick={() => handlePlayAudio(s.partition_key, s.audio_file!)}
                          className={`rounded-sm p-1.5 transition-colors ${playing === audioKey ? "bg-[var(--color-accent-subtle)] text-[var(--color-accent)]" : "text-[var(--color-text-muted)] hover:bg-[var(--color-bg-tertiary)]"}`}
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
                      <a href={api.downloadShowURL(s.partition_key, s.base_name)} className="rounded-sm p-1.5 text-[var(--color-text-muted)] transition-colors hover:bg-[var(--color-bg-tertiary)]">
                        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                        </svg>
                      </a>
                      <button onClick={() => handleDelete(s.partition_key, s.base_name)} className="rounded-sm p-1.5 text-[var(--color-danger)] transition-colors hover:bg-[var(--color-danger-bg)]">
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
