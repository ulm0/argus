"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { CameraName, VideoEvent } from "@/lib/types";
import { CAMERA_LABELS } from "@/lib/types";
import { DashcamMP4, findSeiAtTime } from "@/lib/sei-parser";
import type { SeiFrame, SeiMetadata } from "@/lib/sei-parser";
import ArgusHUD from "./ArgusHUD";

interface DashcamPlayerProps {
  event: VideoEvent;
  streamUrlFn: (cameraFile: string) => string;
  seiUrlFn: (cameraFile: string) => string;
  onDelete?: () => void;
}

const SEEK_STEP = 5;

export default function DashcamPlayer({
  event,
  streamUrlFn,
  seiUrlFn,
  onDelete,
}: DashcamPlayerProps) {
  const cameras = useMemo<CameraName[]>(
    () =>
      (Object.keys(event.camera_videos) as CameraName[]).sort(
        (a, b) => cameraOrder(a) - cameraOrder(b),
      ),
    [event.camera_videos],
  );

  const clips = useMemo(() => event.clips ?? [], [event.clips]);

  const [activeCamera, setActiveCamera] = useState<CameraName>(
    cameras.includes("front") ? "front" : cameras[0],
  );
  const [clipIndex, setClipIndex] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isFullscreen, setIsFullscreen] = useState(false);

  // SEI / HUD state
  const [hudEnabled, setHudEnabled] = useState(false);
  const [seiFrames, setSeiFrames] = useState<SeiFrame[]>([]);
  const [currentSei, setCurrentSei] = useState<SeiMetadata | null>(null);
  const [seiLoading, setSeiLoading] = useState(false);
  const [seiProgress, setSeiProgress] = useState("");
  const [blobSrc, setBlobSrc] = useState<string | null>(null);

  const mainVideoRef = useRef<HTMLVideoElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const seekBarRef = useRef<HTMLDivElement>(null);
  const thumbnailVideoRefs = useRef<Map<CameraName, HTMLVideoElement>>(new Map());
  const seiAbortRef = useRef<AbortController | null>(null);

  const activeFile = event.camera_videos[activeCamera];
  const isEncrypted = event.encrypted_videos?.[activeCamera] ?? false;

  const streamSrc = useMemo(() => {
    if (!activeFile || isEncrypted) return "";
    return streamUrlFn(activeFile);
  }, [activeFile, isEncrypted, streamUrlFn]);

  const videoSrc = blobSrc || streamSrc;

  // Restore HUD toggle from localStorage
  useEffect(() => {
    try {
      setHudEnabled(localStorage.getItem("seiOverlayEnabled") === "true");
    } catch {
      // SSR or restricted storage
    }
  }, []);

  // Sync thumbnail videos to main video time
  const syncThumbnails = useCallback((time: number) => {
    thumbnailVideoRefs.current.forEach((video) => {
      if (Math.abs(video.currentTime - time) > 0.5) {
        video.currentTime = time;
      }
    });
  }, []);

  // Main video event handlers
  const onTimeUpdate = useCallback(() => {
    const v = mainVideoRef.current;
    if (!v) return;
    setCurrentTime(v.currentTime);
    syncThumbnails(v.currentTime);

    if (seiFrames.length > 0) {
      setCurrentSei(findSeiAtTime(seiFrames, v.currentTime));
    }
  }, [syncThumbnails, seiFrames]);

  const onLoadedMetadata = useCallback(() => {
    const v = mainVideoRef.current;
    if (!v) return;
    setDuration(v.duration);
  }, []);

  const onEnded = useCallback(() => {
    setIsPlaying(false);
    if (clipIndex < clips.length - 1) {
      setClipIndex((i) => i + 1);
    }
  }, [clipIndex, clips.length]);

  // Playback controls
  const togglePlay = useCallback(() => {
    const v = mainVideoRef.current;
    if (!v) return;
    if (v.paused) {
      v.play().then(() => setIsPlaying(true)).catch(() => {});
    } else {
      v.pause();
      setIsPlaying(false);
    }
  }, []);

  const seek = useCallback((seconds: number) => {
    const v = mainVideoRef.current;
    if (!v) return;
    v.currentTime = Math.max(0, Math.min(v.currentTime + seconds, v.duration || 0));
  }, []);

  const seekTo = useCallback((time: number) => {
    const v = mainVideoRef.current;
    if (!v) return;
    v.currentTime = time;
  }, []);

  const toggleFullscreen = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    } else {
      el.requestFullscreen().catch(() => {});
    }
  }, []);

  // Fullscreen change detection
  useEffect(() => {
    const onFsChange = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", onFsChange);
    return () => document.removeEventListener("fullscreenchange", onFsChange);
  }, []);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (
        e.target instanceof HTMLInputElement ||
        e.target instanceof HTMLTextAreaElement
      )
        return;

      switch (e.key) {
        case " ":
          e.preventDefault();
          togglePlay();
          break;
        case "ArrowLeft":
          e.preventDefault();
          seek(-SEEK_STEP);
          break;
        case "ArrowRight":
          e.preventDefault();
          seek(SEEK_STEP);
          break;
        case "f":
        case "F":
          e.preventDefault();
          toggleFullscreen();
          break;
      }
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [togglePlay, seek, toggleFullscreen]);

  // Seek bar click
  const handleSeekBarClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      const bar = seekBarRef.current;
      if (!bar || !duration) return;
      const rect = bar.getBoundingClientRect();
      const pct = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
      seekTo(pct * duration);
    },
    [duration, seekTo],
  );

  // ── SEI download & parse ──

  const abortSei = useCallback(() => {
    if (seiAbortRef.current) {
      seiAbortRef.current.abort();
      seiAbortRef.current = null;
    }
  }, []);

  const loadSeiData = useCallback(async (file: string) => {
    abortSei();

    const url = seiUrlFn(file);
    setSeiLoading(true);
    setSeiProgress("Downloading video file...");
    setSeiFrames([]);
    setCurrentSei(null);

    const controller = new AbortController();
    seiAbortRef.current = controller;

    try {
      const response = await fetch(url, {
        credentials: "same-origin",
        signal: controller.signal,
      });

      if (!response.ok) throw new Error(`HTTP ${response.status}`);

      const total = parseInt(response.headers.get("content-length") || "0", 10);
      const reader = response.body?.getReader();
      if (!reader) throw new Error("No response body");

      const chunks: Uint8Array[] = [];
      let loaded = 0;

      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        loaded += value.length;

        if (total) {
          const pct = Math.round((loaded / total) * 100);
          const mb = (loaded / 1024 / 1024).toFixed(1);
          const totalMb = (total / 1024 / 1024).toFixed(1);
          setSeiProgress(`${mb} MB / ${totalMb} MB (${pct}%)`);
        }
      }

      setSeiProgress("Parsing video data...");

      const buffer = new Uint8Array(loaded);
      let pos = 0;
      for (const chunk of chunks) {
        buffer.set(chunk, pos);
        pos += chunk.length;
      }

      const mp4 = new DashcamMP4(buffer.buffer);
      const frames = mp4.parseFrames();
      setSeiFrames(frames);

      const blob = new Blob([buffer], { type: "video/mp4" });
      const blobUrl = URL.createObjectURL(blob);

      const v = mainVideoRef.current;
      const savedTime = v?.currentTime ?? 0;
      const wasPlaying = v ? !v.paused : false;

      setBlobSrc(blobUrl);

      // Restore playback state after source swap
      requestAnimationFrame(() => {
        const vid = mainVideoRef.current;
        if (vid) {
          vid.currentTime = savedTime;
          if (wasPlaying) vid.play().catch(() => {});
        }
      });

      const seiCount = frames.filter((f) => f.sei).length;
      setSeiProgress(`${seiCount} telemetry frames loaded`);
      setTimeout(() => setSeiLoading(false), 800);
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === "AbortError") {
        setSeiLoading(false);
        return;
      }
      setSeiProgress(`Download failed: ${err instanceof Error ? err.message : "unknown error"}`);
      setTimeout(() => setSeiLoading(false), 2000);
    }
  }, [abortSei, seiUrlFn]);

  // Toggle HUD
  const toggleHud = useCallback(() => {
    setHudEnabled((prev) => {
      const next = !prev;
      try { localStorage.setItem("seiOverlayEnabled", String(next)); } catch {}

      if (next && activeFile && !isEncrypted) {
        loadSeiData(activeFile);
      } else if (!next) {
        abortSei();
        setSeiFrames([]);
        setCurrentSei(null);
        setSeiLoading(false);

        if (blobSrc) {
          const v = mainVideoRef.current;
          const savedTime = v?.currentTime ?? 0;
          const wasPlaying = v ? !v.paused : false;
          URL.revokeObjectURL(blobSrc);
          setBlobSrc(null);
          requestAnimationFrame(() => {
            const vid = mainVideoRef.current;
            if (vid) {
              vid.currentTime = savedTime;
              if (wasPlaying) vid.play().catch(() => {});
            }
          });
        }
      }

      return next;
    });
  }, [activeFile, isEncrypted, loadSeiData, abortSei, blobSrc]);

  // Reset SEI on camera switch
  const switchCamera = useCallback((cam: CameraName) => {
    abortSei();
    setSeiFrames([]);
    setCurrentSei(null);
    setSeiLoading(false);

    if (blobSrc) {
      URL.revokeObjectURL(blobSrc);
      setBlobSrc(null);
    }

    const mainV = mainVideoRef.current;
    const savedTime = mainV?.currentTime ?? 0;
    const wasPlaying = mainV ? !mainV.paused : false;

    setActiveCamera(cam);

    requestAnimationFrame(() => {
      const v = mainVideoRef.current;
      if (v) {
        v.currentTime = savedTime;
        if (wasPlaying) v.play().catch(() => {});
      }

      // Re-trigger SEI load if HUD is enabled
      const file = event.camera_videos[cam];
      const encrypted = event.encrypted_videos?.[cam] ?? false;
      if (hudEnabled && file && !encrypted) {
        loadSeiData(file);
      }
    });
  }, [abortSei, blobSrc, event.camera_videos, event.encrypted_videos, hudEnabled, loadSeiData]);

  // Cleanup blob URLs on unmount
  useEffect(() => {
    return () => {
      abortSei();
      if (blobSrc) URL.revokeObjectURL(blobSrc);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div
      ref={containerRef}
      className={`flex flex-col w-full bg-black ${isFullscreen ? "h-screen" : ""}`}
    >
      {/* Main video */}
      <div className="relative w-full bg-black" style={{ aspectRatio: "16/9" }}>
        {isEncrypted ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-zinc-400">
            <LockIcon />
            <p className="text-sm">Encrypted video</p>
          </div>
        ) : videoSrc ? (
          <video
            ref={mainVideoRef}
            src={videoSrc}
            className="h-full w-full object-contain"
            onTimeUpdate={onTimeUpdate}
            onLoadedMetadata={onLoadedMetadata}
            onEnded={onEnded}
            onPlay={() => setIsPlaying(true)}
            onPause={() => setIsPlaying(false)}
            playsInline
          />
        ) : (
          <div className="absolute inset-0 flex items-center justify-center text-zinc-500 text-sm">
            No video available
          </div>
        )}

        {/* Tesla HUD Overlay */}
        <ArgusHUD sei={currentSei} visible={hudEnabled && seiFrames.length > 0} />

        {/* SEI loading overlay */}
        {seiLoading && (
          <div className="absolute inset-0 z-30 flex items-center justify-center bg-black/70">
            <div className="flex flex-col items-center gap-3 rounded-lg bg-black/90 px-8 py-5 text-white backdrop-blur-sm">
              <div className="h-6 w-6 animate-spin rounded-full border-2 border-white/30 border-t-white" />
              <span className="text-sm">{seiProgress}</span>
            </div>
          </div>
        )}

        {/* Overlay play button */}
        {!isPlaying && !isEncrypted && videoSrc && !seiLoading && (
          <button
            onClick={togglePlay}
            className="absolute inset-0 flex items-center justify-center bg-black/20 transition-opacity hover:bg-black/30"
            aria-label="Play"
          >
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-white/90 shadow-lg">
              <svg className="ml-1 h-7 w-7 text-zinc-900" fill="currentColor" viewBox="0 0 24 24">
                <path d="M8 5v14l11-7z" />
              </svg>
            </div>
          </button>
        )}
      </div>

      {/* Controls */}
      <div className="bg-zinc-900 px-3 py-2 space-y-2">
        {/* Seek bar */}
        <div
          ref={seekBarRef}
          onClick={handleSeekBarClick}
          className="group relative h-1.5 w-full cursor-pointer rounded-full bg-zinc-700 transition-all hover:h-2.5"
        >
          <div
            className="h-full rounded-full bg-[var(--color-accent)]"
            style={{ width: `${duration ? (currentTime / duration) * 100 : 0}%` }}
          />
          <div
            className="absolute top-1/2 -translate-y-1/2 h-3.5 w-3.5 rounded-full bg-[var(--color-accent)] shadow opacity-0 group-hover:opacity-100 transition-opacity"
            style={{ left: `calc(${duration ? (currentTime / duration) * 100 : 0}% - 7px)` }}
          />
        </div>

        {/* Button row */}
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {/* Prev clip */}
            {clips.length > 1 && (
              <button
                onClick={() => setClipIndex((i) => Math.max(0, i - 1))}
                disabled={clipIndex === 0}
                className="rounded p-1.5 text-zinc-300 hover:text-white disabled:opacity-30 transition-colors"
                aria-label="Previous clip"
              >
                <svg className="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M6 6h2v12H6zm3.5 6l8.5 6V6z" />
                </svg>
              </button>
            )}

            {/* Play/Pause */}
            <button
              onClick={togglePlay}
              className="rounded p-1.5 text-zinc-300 hover:text-white transition-colors"
              aria-label={isPlaying ? "Pause" : "Play"}
            >
              {isPlaying ? (
                <svg className="h-6 w-6" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" />
                </svg>
              ) : (
                <svg className="h-6 w-6" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M8 5v14l11-7z" />
                </svg>
              )}
            </button>

            {/* Next clip */}
            {clips.length > 1 && (
              <button
                onClick={() => setClipIndex((i) => Math.min(clips.length - 1, i + 1))}
                disabled={clipIndex === clips.length - 1}
                className="rounded p-1.5 text-zinc-300 hover:text-white disabled:opacity-30 transition-colors"
                aria-label="Next clip"
              >
                <svg className="h-5 w-5" fill="currentColor" viewBox="0 0 24 24">
                  <path d="M6 18l8.5-6L6 6v12zm10-12v12h2V6h-2z" />
                </svg>
              </button>
            )}

            {/* Time display */}
            <span className="ml-2 text-xs tabular-nums text-zinc-400">
              {formatTime(currentTime)} / {formatTime(duration)}
            </span>

            {clips.length > 1 && (
              <span className="text-xs text-zinc-500 ml-1">
                Clip {clipIndex + 1}/{clips.length}
              </span>
            )}
          </div>

          <div className="flex items-center gap-1">
            {/* HUD Overlay toggle */}
            <button
              onClick={toggleHud}
              className={`
                rounded p-1.5 transition-colors
                ${hudEnabled
                  ? "text-[var(--color-accent)]"
                  : "text-zinc-400 hover:text-white"
                }
              `}
              aria-label="Toggle HUD Overlay"
              title="HUD Overlay"
            >
              <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
                <rect x="2" y="3" width="20" height="14" rx="2" />
                <line x1="8" y1="21" x2="16" y2="21" />
                <line x1="12" y1="17" x2="12" y2="21" />
                <circle cx="12" cy="10" r="2" />
                <path d="M7 10h2M15 10h2" />
              </svg>
            </button>

            {onDelete && (
              <button
                onClick={onDelete}
                className="rounded p-1.5 text-zinc-400 hover:text-[var(--color-danger)] transition-colors"
                aria-label="Delete event"
              >
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                </svg>
              </button>
            )}
            <button
              onClick={toggleFullscreen}
              className="rounded p-1.5 text-zinc-300 hover:text-white transition-colors"
              aria-label="Fullscreen"
            >
              {isFullscreen ? (
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 9V4.5M9 9H4.5M9 9L3.75 3.75M9 15v4.5M9 15H4.5M9 15l-5.25 5.25M15 9h4.5M15 9V4.5M15 9l5.25-5.25M15 15h4.5M15 15v4.5M15 15l5.25 5.25" />
                </svg>
              ) : (
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 3.75v4.5m0-4.5h4.5m-4.5 0L9 9M3.75 20.25v-4.5m0 4.5h4.5m-4.5 0L9 15M20.25 3.75h-4.5m4.5 0v4.5m0-4.5L15 9m5.25 11.25h-4.5m4.5 0v-4.5m0 4.5L15 15" />
                </svg>
              )}
            </button>
          </div>
        </div>
      </div>

      {/* Camera thumbnails */}
      {cameras.length > 1 && (
        <div className="bg-zinc-900 px-3 pb-3">
          <div className="grid gap-2" style={{ gridTemplateColumns: `repeat(${Math.min(cameras.length, 6)}, 1fr)` }}>
            {cameras.map((cam) => {
              const file = event.camera_videos[cam];
              const encrypted = event.encrypted_videos?.[cam] ?? false;
              const isActive = cam === activeCamera;

              return (
                <button
                  key={cam}
                  onClick={() => switchCamera(cam)}
                  className={`
                    relative overflow-hidden rounded-sm aspect-video transition-all
                    ${isActive
                      ? "ring-2 ring-[var(--color-accent)] ring-offset-1 ring-offset-zinc-900"
                      : "opacity-60 hover:opacity-90"
                    }
                  `}
                >
                  {encrypted ? (
                    <div className="absolute inset-0 flex flex-col items-center justify-center bg-zinc-800 text-zinc-500 text-xs gap-0.5">
                      <LockIcon className="h-4 w-4" />
                      <span>Encrypted</span>
                    </div>
                  ) : file ? (
                    <video
                      ref={(el) => {
                        if (el) thumbnailVideoRefs.current.set(cam, el);
                        else thumbnailVideoRefs.current.delete(cam);
                      }}
                      src={streamUrlFn(file)}
                      className="h-full w-full object-cover"
                      muted
                      playsInline
                      preload="metadata"
                    />
                  ) : (
                    <div className="absolute inset-0 flex items-center justify-center bg-zinc-800 text-zinc-600 text-xs">
                      N/A
                    </div>
                  )}
                  <span className="absolute bottom-0 inset-x-0 bg-gradient-to-t from-black/80 to-transparent px-1.5 py-1 text-[10px] font-medium text-white text-center">
                    {CAMERA_LABELS[cam] ?? cam}
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

function LockIcon({ className = "h-6 w-6" }: { className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 10.5V6.75a4.5 4.5 0 10-9 0v3.75m-.75 11.25h10.5a2.25 2.25 0 002.25-2.25v-6.75a2.25 2.25 0 00-2.25-2.25H6.75a2.25 2.25 0 00-2.25 2.25v6.75a2.25 2.25 0 002.25 2.25z" />
    </svg>
  );
}

function cameraOrder(cam: CameraName): number {
  const order: Record<CameraName, number> = {
    front: 0,
    left_repeater: 1,
    right_repeater: 2,
    back: 3,
    left_pillar: 4,
    right_pillar: 5,
  };
  return order[cam] ?? 99;
}

function formatTime(seconds: number): string {
  if (!isFinite(seconds) || seconds < 0) return "0:00";
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${s.toString().padStart(2, "0")}`;
}
