"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import * as api from "@/lib/api";
import type { FileInfo, DirInfo } from "@/lib/types";
import { useFeatureGuard } from "@/hooks/useFeatureGuard";

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

const CHUNK_SIZE = 1024 * 1024;

export default function MusicPage() {
  const { available, loading: featureLoading } = useFeatureGuard("music_available");
  const [dirs, setDirs] = useState<DirInfo[]>([]);
  const [files, setFiles] = useState<FileInfo[]>([]);
  const [currentPath, setCurrentPath] = useState("");
  const [totalBytes, setTotalBytes] = useState(0);
  const [freeBytes, setFreeBytes] = useState(0);
  const [usedBytes, setUsedBytes] = useState(0);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<Record<string, number>>({});
  const [dragOver, setDragOver] = useState(false);
  const [playing, setPlaying] = useState<string | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const [showMkdir, setShowMkdir] = useState(false);
  const [newDirName, setNewDirName] = useState("");
  const [renaming, setRenaming] = useState<string | null>(null);
  const [newName, setNewName] = useState("");

  const audioRef = useRef<HTMLAudioElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const showToast = useCallback((msg: string, ok = true) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  }, []);

  const loadDir = useCallback(
    async (path: string) => {
      setLoading(true);
      try {
        const res = await api.listFiles(path);
        setDirs(res.dirs || []);
        setFiles(res.files || []);
        setCurrentPath(res.current_path || path);
        setTotalBytes(res.total_bytes);
        setFreeBytes(res.free_bytes);
        setUsedBytes(res.used_bytes);
      } catch {
        showToast("Failed to load directory", false);
      } finally {
        setLoading(false);
      }
    },
    [showToast],
  );

  useEffect(() => {
    loadDir("");
  }, [loadDir]);

  const navigate = (path: string) => {
    setPlaying(null);
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current = null;
    }
    loadDir(path);
  };

  const breadcrumbs = currentPath ? currentPath.split("/").filter(Boolean) : [];

  const handlePlay = (filePath: string) => {
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current = null;
    }
    if (playing === filePath) {
      setPlaying(null);
      return;
    }
    const audio = new Audio(api.playURL(filePath));
    audio.onended = () => setPlaying(null);
    audio.play();
    audioRef.current = audio;
    setPlaying(filePath);
  };

  const handleUploadChunked = async (file: File) => {
    const totalChunks = Math.ceil(file.size / CHUNK_SIZE);
    let uploadId = "";

    for (let i = 0; i < totalChunks; i++) {
      const start = i * CHUNK_SIZE;
      const end = Math.min(start + CHUNK_SIZE, file.size);
      const chunk = file.slice(start, end);

      const res = await api.uploadChunk(chunk, uploadId, file.name, i, totalChunks, currentPath);
      uploadId = res.upload_id;

      setUploadProgress((prev) => ({
        ...prev,
        [file.name]: Math.round(((i + 1) / totalChunks) * 100),
      }));

      if (res.complete) break;
    }
  };

  const handleUpload = async (fileList: FileList | File[]) => {
    if (!fileList.length) return;
    setUploading(true);
    setUploadProgress({});

    try {
      for (const file of Array.from(fileList)) {
        if (file.size > CHUNK_SIZE * 2) {
          await handleUploadChunked(file);
        } else {
          const form = new FormData();
          form.append("file", file);
          form.append("path", currentPath);
          await fetch("/api/music/upload", { method: "POST", body: form });
          setUploadProgress((prev) => ({ ...prev, [file.name]: 100 }));
        }
      }
      showToast(`Uploaded ${fileList.length} file(s)`);
      await loadDir(currentPath);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Upload failed", false);
    } finally {
      setUploading(false);
      setUploadProgress({});
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    handleUpload(e.dataTransfer.files);
  };

  const handleDeleteFile = async (filePath: string) => {
    if (!confirm(`Delete this file?`)) return;
    try {
      await api.deleteFile(filePath);
      showToast("Deleted");
      await loadDir(currentPath);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Delete failed", false);
    }
  };

  const handleDeleteDir = async (dirPath: string) => {
    if (!confirm(`Delete this folder and all contents?`)) return;
    try {
      await api.deleteDir(dirPath);
      showToast("Deleted");
      await loadDir(currentPath);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Delete failed", false);
    }
  };

  const handleMkdir = async () => {
    if (!newDirName.trim()) return;
    try {
      await api.mkdir(currentPath, newDirName.trim());
      showToast("Folder created");
      setNewDirName("");
      setShowMkdir(false);
      await loadDir(currentPath);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const handleRename = async (source: string) => {
    if (!newName.trim()) {
      setRenaming(null);
      return;
    }
    try {
      await api.moveFile(source, currentPath, newName.trim());
      showToast("Renamed");
      setRenaming(null);
      await loadDir(currentPath);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed", false);
    }
  };

  const isAudio = (name: string) => /\.(mp3|wav|flac|aac|ogg|m4a|wma)$/i.test(name);
  const usedPercent = totalBytes > 0 ? (usedBytes / totalBytes) * 100 : 0;

  if (!featureLoading && !available) {
    return (
      <div className="flex min-h-full flex-col items-center justify-center gap-4 p-8 text-center">
        <svg className="h-12 w-12 text-[var(--color-text-muted)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
        </svg>
        <h1 className="text-xl font-bold text-[var(--color-text-primary)]">Music Disabled</h1>
        <p className="max-w-sm text-sm text-[var(--color-text-muted)]">
          The Music partition is not enabled. Set <code className="rounded bg-[var(--color-bg-tertiary)] px-1">music_enabled: true</code> in your <code className="rounded bg-[var(--color-bg-tertiary)] px-1">config.yaml</code> and restart Argus.
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

      <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Music</h1>

      {/* Storage Usage */}
      {totalBytes > 0 && (
          <div className="rounded bg-[var(--color-bg-card)] p-4 shadow-sm">
          <div className="flex items-center justify-between text-sm">
            <span className="text-[var(--color-text-secondary)]">{formatBytes(usedBytes)} used of {formatBytes(totalBytes)}</span>
            <span className="text-[var(--color-text-muted)]">{formatBytes(freeBytes)} free</span>
          </div>
          <div className="mt-2 h-2 overflow-hidden rounded-full bg-[var(--color-border)]">
            <div className={`h-full rounded-full transition-all ${usedPercent > 90 ? "bg-[var(--color-danger)]" : usedPercent > 70 ? "bg-[var(--color-warning)]" : "bg-[var(--color-accent)]"}`} style={{ width: `${usedPercent}%` }} />
          </div>
        </div>
      )}

      {/* Breadcrumbs */}
      <div className="flex flex-wrap items-center gap-1 text-sm">
        <button onClick={() => navigate("")} className="rounded px-1.5 py-0.5 font-medium text-[var(--color-accent)] hover:bg-[var(--color-accent-subtle)]">Music</button>
        {breadcrumbs.map((seg, i) => {
          const path = breadcrumbs.slice(0, i + 1).join("/");
          return (
            <span key={path} className="flex items-center gap-1">
              <span className="text-[var(--color-text-muted)]">/</span>
              <button onClick={() => navigate(path)} className="rounded px-1.5 py-0.5 font-medium text-[var(--color-accent)] hover:bg-[var(--color-accent-subtle)]">{seg}</button>
            </span>
          );
        })}
      </div>

      {/* Upload & Actions */}
      <div className="flex flex-wrap gap-3">
        <div
          onDragOver={(e) => { e.preventDefault(); setDragOver(true); }}
          onDragLeave={() => setDragOver(false)}
          onDrop={handleDrop}
          onClick={() => fileInputRef.current?.click()}
          className={`flex cursor-pointer items-center gap-2 rounded-sm border-2 border-dashed px-4 py-2 text-sm font-medium transition-colors ${dragOver ? "border-[var(--color-accent)] bg-[var(--color-accent-subtle)] text-[var(--color-accent)]" : "border-[var(--color-border)] text-[var(--color-text-secondary)] hover:border-[var(--color-text-muted)]"}`}
        >
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" /></svg>
          {uploading ? "Uploading..." : "Upload Files"}
          <input ref={fileInputRef} type="file" multiple className="hidden" onChange={(e) => e.target.files && handleUpload(e.target.files)} />
        </div>
        <button onClick={() => setShowMkdir(!showMkdir)} className="flex items-center gap-2 rounded-sm border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition-colors hover:bg-[var(--color-bg-tertiary)]">
          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 13h6m-3-3v6m-9 1V7a2 2 0 012-2h6l2 2h6a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z" /></svg>
          New Folder
        </button>
      </div>

      {/* Upload Progress */}
      {Object.keys(uploadProgress).length > 0 && (
        <div className="space-y-2 rounded-sm bg-[var(--color-bg-card-nested)] p-3">
          {Object.entries(uploadProgress).map(([name, pct]) => (
            <div key={name} className="space-y-1">
              <div className="flex justify-between text-xs">
                <span className="truncate text-[var(--color-text-secondary)]">{name}</span>
                <span className="text-[var(--color-text-muted)]">{pct}%</span>
              </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-[var(--color-border)]">
                <div className="h-full rounded-full bg-[var(--color-accent)] transition-all" style={{ width: `${pct}%` }} />
              </div>
            </div>
          ))}
        </div>
      )}

      {showMkdir && (
        <div className="flex gap-2">
          <input type="text" placeholder="Folder name" value={newDirName} onChange={(e) => setNewDirName(e.target.value)} onKeyDown={(e) => e.key === "Enter" && handleMkdir()} autoFocus className="flex-1 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-primary)] px-3 py-2 text-sm text-[var(--color-text-primary)]" />
          <button onClick={handleMkdir} className="rounded bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--color-accent-hover)]">Create</button>
          <button onClick={() => { setShowMkdir(false); setNewDirName(""); }} className="rounded-sm border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]">Cancel</button>
        </div>
      )}

      {/* File List */}
      {loading ? (
        <div className="flex justify-center py-12">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
        </div>
      ) : (
        <div className="overflow-hidden rounded bg-[var(--color-bg-card)] shadow-sm">
          {dirs.length === 0 && files.length === 0 ? (
            <div className="py-12 text-center text-sm text-[var(--color-text-muted)]">Empty directory</div>
          ) : (
            <div className="divide-y divide-[var(--color-border)]">
              {dirs.map((d) => (
                <div key={d.path} className="flex items-center justify-between px-4 py-3 hover:bg-[var(--color-bg-tertiary)]">
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    <span className="shrink-0 text-[var(--color-text-muted)]">
                      <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z" /></svg>
                    </span>
                    <button onClick={() => navigate(d.path)} className="min-w-0 truncate text-left text-sm font-medium text-[var(--color-accent)] hover:underline">
                      {d.name}
                    </button>
                  </div>
                  <button onClick={() => handleDeleteDir(d.path)} className="rounded-sm p-1.5 text-[var(--color-danger)] transition-colors hover:bg-[var(--color-danger-bg)]">
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                  </button>
                </div>
              ))}
              {files.map((f) => (
                <div key={f.path} className="flex items-center justify-between px-4 py-3 hover:bg-[var(--color-bg-tertiary)]">
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    <span className="shrink-0 text-[var(--color-text-muted)]">
                      <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="M9 9l10.5-3m0 6.553v3.75a2.25 2.25 0 01-1.632 2.163l-1.32.377a1.803 1.803 0 11-.99-3.467l2.31-.66a2.25 2.25 0 001.632-2.163zm0 0V2.25L9 5.25v10.303m0 0v3.75a2.25 2.25 0 01-1.632 2.163l-1.32.377a1.803 1.803 0 01-.99-3.467l2.31-.66A2.25 2.25 0 009 15.553z" /></svg>
                    </span>
                    {renaming === f.path ? (
                      <input type="text" value={newName} onChange={(e) => setNewName(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") handleRename(f.path); if (e.key === "Escape") setRenaming(null); }} onBlur={() => handleRename(f.path)} autoFocus className="min-w-0 flex-1 rounded border border-[var(--color-accent)] bg-[var(--color-bg-primary)] px-2 py-0.5 text-sm text-[var(--color-text-primary)]" />
                    ) : (
                      <span className="min-w-0 truncate text-sm text-[var(--color-text-primary)]">{f.name}</span>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <span className="text-xs tabular-nums text-[var(--color-text-muted)]">{formatBytes(f.size)}</span>
                    <div className="flex items-center gap-0.5">
                      {isAudio(f.name) && (
                        <button onClick={() => handlePlay(f.path)} className={`rounded-sm p-1.5 transition-colors ${playing === f.path ? "bg-[var(--color-accent-subtle)] text-[var(--color-accent)]" : "text-[var(--color-text-muted)] hover:bg-[var(--color-bg-tertiary)]"}`}>
                          <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            {playing === f.path ? <path strokeLinecap="round" strokeLinejoin="round" d="M6 4h4v16H6zM14 4h4v16h-4z" /> : <path strokeLinecap="round" strokeLinejoin="round" d="M5 3l14 9-14 9V3z" />}
                          </svg>
                        </button>
                      )}
                      <button onClick={() => { setRenaming(f.path); setNewName(f.name); }} className="rounded-sm p-1.5 text-[var(--color-text-muted)] transition-colors hover:bg-[var(--color-bg-tertiary)]">
                        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
                      </button>
                      <button onClick={() => handleDeleteFile(f.path)} className="rounded-sm p-1.5 text-[var(--color-danger)] transition-colors hover:bg-[var(--color-danger-bg)]">
                        <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
