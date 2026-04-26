"use client";

import { useCallback, useRef, useState } from "react";
import { DashcamMP4, findSeiAtTime, downloadCsv } from "@/lib/sei-parser";
import type { SeiFrame, SeiMetadata } from "@/lib/sei-parser";
import ArgusHUD from "@/components/ArgusHUD";

interface LocalFile {
  name: string;
  url: string;
  frames: SeiFrame[];
}

export default function LocalAnalysisPage() {
  const [files, setFiles] = useState<LocalFile[]>([]);
  const [active, setActive] = useState<LocalFile | null>(null);
  const [currentSei, setCurrentSei] = useState<SeiMetadata | null>(null);
  const [parsing, setParsing] = useState(false);
  const [parseError, setParseError] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);

  const processFiles = useCallback(async (rawFiles: File[]) => {
    const mp4Files = rawFiles.filter((f) => f.name.toLowerCase().endsWith(".mp4"));
    if (mp4Files.length === 0) return;

    setParsing(true);
    setParseError(null);

    const results: LocalFile[] = [];
    for (const file of mp4Files) {
      try {
        const buffer = await file.arrayBuffer();
        const frames = new DashcamMP4(buffer).parseFrames();
        results.push({ name: file.name, url: URL.createObjectURL(file), frames });
      } catch {
        // skip unparseable files
      }
    }

    if (results.length === 0) {
      setParseError("No parseable MP4 files found.");
      setParsing(false);
      return;
    }

    setFiles((prev) => { prev.forEach((f) => URL.revokeObjectURL(f.url)); return results; });
    setActive(results[0]);
    setCurrentSei(null);
    setParsing(false);
  }, []);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
    processFiles(Array.from(e.dataTransfer.files));
  }, [processFiles]);

  const onFileInput = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    processFiles(Array.from(e.target.files ?? []));
    e.target.value = "";
  }, [processFiles]);

  const onTimeUpdate = useCallback(() => {
    const v = videoRef.current;
    if (!v || !active) return;
    setCurrentSei(findSeiAtTime(active.frames, v.currentTime));
  }, [active]);

  if (active) {
    const seiCount = active.frames.filter((f) => f.sei).length;
    return (
      <div className="flex flex-col w-full gap-4 p-4 lg:p-6">
        <div className="flex items-center gap-3 flex-wrap">
          <button
            onClick={() => { files.forEach((f) => URL.revokeObjectURL(f.url)); setFiles([]); setActive(null); setCurrentSei(null); }}
            className="text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)] transition-colors"
          >
            ← Load different files
          </button>
          <span className="text-sm font-medium text-[var(--color-text-primary)] truncate max-w-xs">{active.name}</span>
          {seiCount > 0 && (
            <button
              onClick={() => downloadCsv(active.frames, active.name.replace(/\.mp4$/i, "") + ".csv")}
              className="ml-auto rounded-sm border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)] transition-colors"
            >
              Export CSV ({seiCount} frames)
            </button>
          )}
        </div>

        <div className="relative overflow-hidden rounded-lg bg-black" style={{ aspectRatio: "16/9" }}>
          <video
            ref={videoRef}
            src={active.url}
            className="h-full w-full object-contain"
            controls
            onTimeUpdate={onTimeUpdate}
            playsInline
          />
          <ArgusHUD sei={currentSei} visible={seiCount > 0} />
        </div>

        {files.length > 1 && (
          <div className="flex flex-wrap gap-2">
            {files.map((f) => (
              <button
                key={f.name}
                onClick={() => { setActive(f); setCurrentSei(null); }}
                className={`rounded px-3 py-1 text-sm transition-colors ${
                  f === active
                    ? "bg-[var(--color-accent)] text-white"
                    : "border border-[var(--color-border)] text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]"
                }`}
              >
                {f.name}
              </button>
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center min-h-full gap-6 p-8">
      <div className="text-center space-y-1">
        <h1 className="text-xl font-semibold text-[var(--color-text-primary)]">Local File Analysis</h1>
        <p className="text-sm text-[var(--color-text-muted)]">
          Analyze dashcam MP4 files directly from your computer — no upload required.
        </p>
      </div>

      <label
        className={`relative flex flex-col items-center justify-center w-full max-w-lg h-48 rounded-xl border-2 border-dashed transition-colors cursor-pointer ${
          isDragging
            ? "border-[var(--color-accent)] bg-[var(--color-accent-subtle)]"
            : "border-[var(--color-border)] hover:border-[var(--color-accent)] hover:bg-[var(--color-bg-secondary)]"
        }`}
        onDragOver={(e) => { e.preventDefault(); setIsDragging(true); }}
        onDragLeave={() => setIsDragging(false)}
        onDrop={onDrop}
      >
        <input type="file" accept=".mp4" multiple className="sr-only" onChange={onFileInput} />
        {parsing ? (
          <div className="flex flex-col items-center gap-3">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-[var(--color-border)] border-t-[var(--color-accent)]" />
            <span className="text-sm text-[var(--color-text-muted)]">Parsing files…</span>
          </div>
        ) : (
          <div className="flex flex-col items-center gap-2 text-center px-4">
            <svg className="h-10 w-10 text-[var(--color-text-muted)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5m-13.5-9L12 3m0 0l4.5 4.5M12 3v13.5" />
            </svg>
            <p className="text-sm font-medium text-[var(--color-text-primary)]">Drop MP4 files here</p>
            <p className="text-xs text-[var(--color-text-muted)]">or click to browse</p>
          </div>
        )}
      </label>

      {parseError && <p className="text-sm text-[var(--color-danger)]">{parseError}</p>}

      <p className="text-xs text-[var(--color-text-muted)] max-w-sm text-center">
        Files are processed entirely in your browser. Nothing is uploaded.
        SEI telemetry (speed, gear, GPS, etc.) is shown as a live HUD overlay.
      </p>
    </div>
  );
}
