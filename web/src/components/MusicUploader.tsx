"use client";

import { useCallback, useRef, useState } from "react";
import { formatBytes } from "./FileSize";

type FileStatus = "pending" | "uploading" | "complete" | "error";

interface QueuedFile {
  id: string;
  file: File;
  relativePath: string;
  status: FileStatus;
  progress: number;
  error?: string;
  uploadId?: string;
}

interface MusicUploaderProps {
  uploadUrl?: string;
  chunkSizeMB?: number;
  currentPath?: string;
  onUploadComplete?: () => void;
}

let fileIdCounter = 0;

export default function MusicUploader({
  uploadUrl = "/api/music/upload-chunk",
  chunkSizeMB = 2,
  currentPath = "",
  onUploadComplete,
}: MusicUploaderProps) {
  const [queue, setQueue] = useState<QueuedFile[]>([]);
  const [isDragOver, setIsDragOver] = useState(false);
  const [isUploading, setIsUploading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  const chunkSize = chunkSizeMB * 1024 * 1024;

  const enqueueFiles = useCallback(
    (files: File[], basePath = "") => {
      const newItems: QueuedFile[] = files.map((file) => ({
        id: `file-${++fileIdCounter}`,
        file,
        relativePath: basePath
          ? `${basePath}/${file.webkitRelativePath || file.name}`
          : file.name,
        status: "pending" as FileStatus,
        progress: 0,
      }));
      setQueue((prev) => [...prev, ...newItems]);
    },
    [],
  );

  const handleDrop = useCallback(
    async (e: React.DragEvent) => {
      e.preventDefault();
      setIsDragOver(false);

      const items = e.dataTransfer.items;
      const files: File[] = [];

      async function traverseEntry(entry: FileSystemEntry, path: string) {
        if (entry.isFile) {
          const fileEntry = entry as FileSystemFileEntry;
          const file = await new Promise<File>((resolve, reject) =>
            fileEntry.file(resolve, reject),
          );
          Object.defineProperty(file, "webkitRelativePath", {
            value: path ? `${path}/${entry.name}` : entry.name,
            writable: false,
          });
          files.push(file);
        } else if (entry.isDirectory) {
          const dirEntry = entry as FileSystemDirectoryEntry;
          const reader = dirEntry.createReader();
          const entries = await new Promise<FileSystemEntry[]>((resolve, reject) =>
            reader.readEntries(resolve, reject),
          );
          for (const child of entries) {
            await traverseEntry(child, path ? `${path}/${entry.name}` : entry.name);
          }
        }
      }

      if (items) {
        const entries: FileSystemEntry[] = [];
        for (let i = 0; i < items.length; i++) {
          const entry = items[i].webkitGetAsEntry?.();
          if (entry) entries.push(entry);
        }
        for (const entry of entries) {
          await traverseEntry(entry, "");
        }
        enqueueFiles(files);
      }
    },
    [enqueueFiles],
  );

  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const fileList = e.target.files;
      if (!fileList) return;
      enqueueFiles(Array.from(fileList));
      e.target.value = "";
    },
    [enqueueFiles],
  );

  const uploadFile = async (item: QueuedFile, signal: AbortSignal): Promise<void> => {
    const totalChunks = Math.max(1, Math.ceil(item.file.size / chunkSize));
    let uploadId = "";

    for (let i = 0; i < totalChunks; i++) {
      if (signal.aborted) throw new DOMException("Aborted", "AbortError");

      const start = i * chunkSize;
      const end = Math.min(start + chunkSize, item.file.size);
      const blob = item.file.slice(start, end);

      const fd = new FormData();
      fd.append("chunk", blob);
      fd.append("filename", item.relativePath.split("/").pop() || item.file.name);
      fd.append("path", currentPath ? `${currentPath}/${dirPart(item.relativePath)}` : dirPart(item.relativePath));
      fd.append("chunk_index", String(i));
      fd.append("total_chunks", String(totalChunks));
      if (uploadId) fd.append("upload_id", uploadId);

      const res = await fetch(uploadUrl, {
        method: "POST",
        body: fd,
        signal,
      });

      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || res.statusText);
      }

      const data = await res.json();
      if (!uploadId && data.upload_id) {
        uploadId = data.upload_id;
      }

      const progress = Math.round(((i + 1) / totalChunks) * 100);
      setQueue((prev) =>
        prev.map((q) =>
          q.id === item.id ? { ...q, progress, uploadId: uploadId || q.uploadId } : q,
        ),
      );
    }
  };

  const startUpload = useCallback(async () => {
    const pending = queue.filter((q) => q.status === "pending");
    if (pending.length === 0) return;

    setIsUploading(true);
    const controller = new AbortController();
    abortRef.current = controller;

    for (const item of pending) {
      if (controller.signal.aborted) break;

      setQueue((prev) =>
        prev.map((q) => (q.id === item.id ? { ...q, status: "uploading" } : q)),
      );

      try {
        await uploadFile(item, controller.signal);
        setQueue((prev) =>
          prev.map((q) =>
            q.id === item.id ? { ...q, status: "complete", progress: 100 } : q,
          ),
        );
      } catch (err) {
        if ((err as Error).name === "AbortError") break;
        setQueue((prev) =>
          prev.map((q) =>
            q.id === item.id
              ? { ...q, status: "error", error: (err as Error).message }
              : q,
          ),
        );
      }
    }

    setIsUploading(false);
    abortRef.current = null;
    onUploadComplete?.();
  }, [queue, chunkSize, uploadUrl, currentPath, onUploadComplete]);

  const cancelUpload = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const clearCompleted = useCallback(() => {
    setQueue((prev) => prev.filter((q) => q.status !== "complete"));
  }, []);

  const removeItem = useCallback((id: string) => {
    setQueue((prev) => prev.filter((q) => q.id !== id));
  }, []);

  const pendingCount = queue.filter((q) => q.status === "pending").length;
  const completedCount = queue.filter((q) => q.status === "complete").length;

  return (
    <div className="w-full space-y-4">
      {/* Drop zone */}
      <div
        onDragOver={(e) => {
          e.preventDefault();
          setIsDragOver(true);
        }}
        onDragLeave={() => setIsDragOver(false)}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
        className={`
          relative flex flex-col items-center justify-center gap-3 rounded-sm border-2 border-dashed
          px-6 py-10 cursor-pointer transition-all duration-200
          ${
            isDragOver
              ? "border-[var(--color-accent)] bg-[var(--color-accent-subtle)] scale-[1.01]"
              : "border-[var(--color-border)] bg-[var(--color-bg-tertiary)] hover:border-[var(--color-text-muted)]"
          }
        `}
      >
        <svg
          className={`h-10 w-10 transition-colors ${isDragOver ? "text-[var(--color-accent)]" : "text-[var(--color-text-muted)]"}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={1.5}
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M12 16.5V9.75m0 0l3 3m-3-3l-3 3M6.75 19.5a4.5 4.5 0 01-1.41-8.775 5.25 5.25 0 0110.233-2.33 3 3 0 013.758 3.848A3.752 3.752 0 0118 19.5H6.75z"
          />
        </svg>
        <div className="text-center">
          <p className="text-sm font-medium text-[var(--color-text-primary)]">
            Drop files or folders here
          </p>
          <p className="mt-1 text-xs text-[var(--color-text-muted)]">
            or click to browse &middot; Chunk size: {chunkSizeMB} MB
          </p>
        </div>
        <input
          ref={inputRef}
          type="file"
          multiple
          onChange={handleFileInput}
          className="hidden"
        />
      </div>

      {/* Action buttons */}
      {queue.length > 0 && (
        <div className="flex items-center gap-2 flex-wrap">
          {pendingCount > 0 && !isUploading && (
            <button
              onClick={startUpload}
              className="rounded bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--color-accent-hover)] transition-colors"
            >
              Upload {pendingCount} file{pendingCount !== 1 ? "s" : ""}
            </button>
          )}
          {isUploading && (
            <button
              onClick={cancelUpload}
              className="rounded bg-[var(--color-danger)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 transition-colors"
            >
              Cancel
            </button>
          )}
          {completedCount > 0 && (
            <button
              onClick={clearCompleted}
              className="rounded border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)] transition-colors"
            >
              Clear completed
            </button>
          )}
        </div>
      )}

      {/* File queue */}
      {queue.length > 0 && (
        <ul className="divide-y divide-[var(--color-border-light)] rounded-sm border border-[var(--color-border)] overflow-hidden">
          {queue.map((item) => (
            <li
              key={item.id}
              className="flex items-center gap-3 px-4 py-3 bg-[var(--color-bg-card)]"
            >
              <StatusIcon status={item.status} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-[var(--color-text-primary)]">
                  {item.relativePath}
                </p>
                <p className="text-xs text-[var(--color-text-muted)]">
                  {formatBytes(item.file.size)}
                  {item.error && (
                    <span className="ml-2 text-[var(--color-danger)]">{item.error}</span>
                  )}
                </p>
                {item.status === "uploading" && (
                  <div className="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-[var(--color-bg-tertiary)]">
                    <div
                      className="h-full rounded-full bg-[var(--color-accent)] transition-all duration-300"
                      style={{ width: `${item.progress}%` }}
                    />
                  </div>
                )}
              </div>
              {item.status !== "uploading" && (
                <button
                  onClick={() => removeItem(item.id)}
                  className="shrink-0 rounded p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)] transition-colors"
                  aria-label="Remove"
                >
                  <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function StatusIcon({ status }: { status: FileStatus }) {
  switch (status) {
    case "pending":
      return (
        <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--color-bg-tertiary)]">
          <span className="h-2 w-2 rounded-full bg-[var(--color-text-muted)]" />
        </span>
      );
    case "uploading":
      return (
        <span className="flex h-6 w-6 shrink-0 items-center justify-center">
          <svg className="h-5 w-5 animate-spin text-[var(--color-accent)]" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
        </span>
      );
    case "complete":
      return (
        <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--color-success-bg)]">
          <svg className="h-3.5 w-3.5 text-[var(--color-success)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        </span>
      );
    case "error":
      return (
        <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--color-danger-bg)]">
          <svg className="h-3.5 w-3.5 text-[var(--color-danger)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </span>
      );
  }
}

function dirPart(relativePath: string): string {
  const parts = relativePath.split("/");
  return parts.length > 1 ? parts.slice(0, -1).join("/") : "";
}
