"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import dynamic from "next/dynamic";
import type { CameraName, VideoEvent } from "@/lib/types";
import { CAMERA_LABELS } from "@/lib/types";
import { findSeiAtTime, downloadCsv } from "@/lib/sei-parser";
import type { SeiFrame, SeiMetadata } from "@/lib/sei-parser";
import { useMapTheme } from "@/lib/useMapTheme";
import ArgusHUD from "./ArgusHUD";
import CornerResize from "./CornerResize";
import { useViewerScale } from "@/lib/useViewerScale";

const MapOverlay = dynamic(() => import("./MapOverlay"), { ssr: false });

// Bounds for overlay scales. Kept in sync with the server-side validation in
// internal/api/handlers/config.go (which accepts a slightly wider envelope).
const HUD_SCALE_MIN = 0.7;
const HUD_SCALE_MAX = 1.8;
const MAP_SCALE_MIN = 0.6;
const MAP_SCALE_MAX = 2.2;

interface DashcamPlayerProps {
  event: VideoEvent;
  streamUrlFn: (cameraFile: string) => string;
  telemetryUrlFn: (cameraFile: string) => string;
  downloadUrlFn?: (cameraFile: string) => string;
  downloadZipUrl?: string;
  onDelete?: () => void;
}

const SEEK_STEP = 5;

// Fixed camera grid positions for composed view (Tesla layout)
const CAMERA_SLOTS: { cam: CameraName; area: string }[] = [
  { cam: "left_repeater", area: "lr" },
  { cam: "front",         area: "front" },
  { cam: "right_repeater",area: "rr" },
  { cam: "left_pillar",   area: "lp" },
  { cam: "back",          area: "back" },
  { cam: "right_pillar",  area: "rp" },
];

export default function DashcamPlayer({
  event,
  streamUrlFn,
  telemetryUrlFn,
  downloadUrlFn,
  downloadZipUrl,
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
  const [clipIndex, setClipIndex] = useState(event.starting_clip_index ?? 0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [viewMode, setViewMode] = useState<"single" | "composed">("composed");
  const [mapTheme] = useMapTheme();

  // SEI / HUD state. Overlay toggles still live in localStorage (per-device
  // UI preference); overlay *scales* are server-persisted so they survive
  // app updates and re-flashes.
  const [hudEnabled, setHudEnabled] = useState(false);
  const [hudScale, updateHudScale] = useViewerScale({
    kind: "hud",
    min: HUD_SCALE_MIN,
    max: HUD_SCALE_MAX,
  });
  const [mapEnabled, setMapEnabled] = useState(true);
  const [mapScale, updateMapScale] = useViewerScale({
    kind: "map",
    min: MAP_SCALE_MIN,
    max: MAP_SCALE_MAX,
  });
  const [seiFrames, setSeiFrames] = useState<SeiFrame[]>([]);
  const [currentSei, setCurrentSei] = useState<SeiMetadata | null>(null);
  const [seiLoading, setSeiLoading] = useState(false);
  const [seiProgress, setSeiProgress] = useState("");

  const mainVideoRef = useRef<HTMLVideoElement | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const seekBarRef = useRef<HTMLDivElement>(null);
  const thumbnailVideoRefs = useRef<Map<CameraName, HTMLVideoElement>>(new Map());
  const seiAbortRef = useRef<AbortController | null>(null);
  // Populated only in composed mode; empty in single mode (makes sync calls no-ops)
  const composedRefs = useRef<Map<CameraName, HTMLVideoElement>>(new Map());

  const activeFile = useMemo(() => {
    const clipTs = clips[clipIndex];
    if (clipTs) {
      const refFile = Object.values(event.camera_videos)[0] ?? "";
      const ext = refFile.match(/\.(\w+)$/)?.[1] ?? "mp4";
      return `${clipTs}-${activeCamera}.${ext}`;
    }
    return event.camera_videos[activeCamera];
  }, [clips, clipIndex, activeCamera, event.camera_videos]);
  const isEncrypted = event.encrypted_videos?.[activeCamera] ?? false;

  const streamSrc = useMemo(() => {
    if (!activeFile || isEncrypted) return "";
    return streamUrlFn(activeFile);
  }, [activeFile, isEncrypted, streamUrlFn]);

  // Master camera for composed view: prefer front, else first unencrypted
  const composedMaster = useMemo<CameraName>(() => {
    const priority: CameraName[] = ["front", "back", "left_repeater", "right_repeater", "left_pillar", "right_pillar"];
    for (const cam of priority) {
      if (cameras.includes(cam) && !event.encrypted_videos?.[cam]) return cam;
    }
    return cameras[0] ?? "front";
  }, [cameras, event.encrypted_videos]);

  // Build the file path for a given camera at the current clip index
  const getFileForCamera = useCallback((cam: CameraName): string => {
    const clipTs = clips[clipIndex];
    if (clipTs) {
      const refFile = Object.values(event.camera_videos)[0] ?? "";
      const ext = refFile.match(/\.(\w+)$/)?.[1] ?? "mp4";
      return `${clipTs}-${cam}.${ext}`;
    }
    return event.camera_videos[cam] ?? "";
  }, [clips, clipIndex, event.camera_videos]);

  // Restore HUD + map toggles from localStorage. Scales are handled by
  // useViewerScale (server-persisted with localStorage cache).
  useEffect(() => {
    try {
      const hudOn = localStorage.getItem("seiOverlayEnabled") !== "false";
      const mapOn = localStorage.getItem("mapOverlayEnabled") !== "false";
      setHudEnabled(hudOn);
      setMapEnabled(mapOn);
      if ((hudOn || mapOn) && activeFile && !isEncrypted) {
        loadTelemetry(activeFile);
      }
    } catch {
      // SSR or restricted storage
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Clear SEI and reload when the active file changes (clip navigation or camera switch)
  const prevFileRef = useRef<string | undefined>(undefined);
  useEffect(() => {
    const prev = prevFileRef.current;
    prevFileRef.current = activeFile;
    if (prev === undefined || prev === activeFile) return;

    abortSei();
    setSeiFrames([]);
    setCurrentSei(null);
    setSeiLoading(false);

    if ((hudEnabled || mapEnabled) && activeFile && !isEncrypted) {
      loadTelemetry(activeFile);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeFile]);

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

    // Keep composed follower cameras in sync
    composedRefs.current.forEach((cv) => {
      if (cv !== v && Math.abs(cv.currentTime - v.currentTime) > 0.5) {
        cv.currentTime = v.currentTime;
      }
    });

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

  // Playback controls — composedRefs is empty in single mode, so the forEach is a no-op
  const togglePlay = useCallback(() => {
    const v = mainVideoRef.current;
    if (!v) return;
    if (v.paused) {
      v.play()
        .then(() => {
          setIsPlaying(true);
          composedRefs.current.forEach((cv) => {
            if (cv !== v) { cv.currentTime = v.currentTime; cv.play().catch(() => {}); }
          });
        })
        .catch(() => {});
    } else {
      v.pause();
      setIsPlaying(false);
      composedRefs.current.forEach((cv) => { if (cv !== v) cv.pause(); });
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
    composedRefs.current.forEach((cv) => { if (cv !== v) cv.currentTime = time; });
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

  // ── Telemetry fetch ──

  const abortSei = useCallback(() => {
    if (seiAbortRef.current) {
      seiAbortRef.current.abort();
      seiAbortRef.current = null;
    }
  }, []);

  const loadTelemetry = useCallback(async (file: string) => {
    abortSei();
    setSeiLoading(true);
    setSeiFrames([]);
    setCurrentSei(null);
    setSeiProgress("Loading telemetry…");

    const controller = new AbortController();
    seiAbortRef.current = controller;

    try {
      const response = await fetch(telemetryUrlFn(file), {
        credentials: "same-origin",
        signal: controller.signal,
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = await response.json() as { frames: SeiFrame[] };
      const frames = data.frames ?? [];
      setSeiFrames(frames);
      setSeiProgress(`${frames.length} telemetry frames`);
      setTimeout(() => setSeiLoading(false), 500);
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === "AbortError") {
        setSeiLoading(false);
        return;
      }
      setSeiProgress(`Failed: ${err instanceof Error ? err.message : "unknown error"}`);
      setTimeout(() => setSeiLoading(false), 2000);
    }
  }, [abortSei, telemetryUrlFn]);

  // Toggle HUD
  const toggleHud = useCallback(() => {
    setHudEnabled((prev) => {
      const next = !prev;
      try { localStorage.setItem("seiOverlayEnabled", String(next)); } catch {}

      if (next && activeFile && !isEncrypted) {
        loadTelemetry(activeFile);
      } else if (!next && !mapEnabled) {
        abortSei();
        setSeiFrames([]);
        setCurrentSei(null);
        setSeiLoading(false);
      }

      return next;
    });
  }, [activeFile, isEncrypted, loadTelemetry, abortSei, mapEnabled]);

  // Toggle map overlay
  const toggleMap = useCallback(() => {
    setMapEnabled((prev) => {
      const next = !prev;
      try { localStorage.setItem("mapOverlayEnabled", String(next)); } catch {}

      if (next && activeFile && !isEncrypted && seiFrames.length === 0) {
        loadTelemetry(activeFile);
      } else if (!next && !hudEnabled) {
        abortSei();
        setSeiFrames([]);
        setCurrentSei(null);
        setSeiLoading(false);
      }

      return next;
    });
  }, [activeFile, isEncrypted, loadTelemetry, abortSei, hudEnabled, seiFrames.length]);

  const handleHudScaleWheel = useCallback((e: React.WheelEvent<HTMLDivElement>) => {
    e.preventDefault();
    const step = e.deltaY < 0 ? 0.1 : -0.1;
    updateHudScale(hudScale + step);
  }, [hudScale, updateHudScale]);

  const handleMapScaleWheel = useCallback((e: React.WheelEvent<HTMLDivElement>) => {
    e.preventDefault();
    const step = e.deltaY < 0 ? 0.1 : -0.1;
    updateMapScale(mapScale + step);
  }, [mapScale, updateMapScale]);

  // Switch camera — no-op in composed mode
  const switchCamera = useCallback((cam: CameraName) => {
    if (viewMode === "composed") return;
    abortSei();
    setSeiFrames([]);
    setCurrentSei(null);
    setSeiLoading(false);

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
    });
  }, [viewMode, abortSei]);

  // Abort any in-flight fetch on unmount
  useEffect(() => {
    return () => { abortSei(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Clear composedRefs when leaving composed mode
  useEffect(() => {
    if (viewMode === "single") {
      composedRefs.current.clear();
    }
  }, [viewMode]);

  return (
    <div
      ref={containerRef}
      className={`flex flex-col w-full bg-black ${isFullscreen ? "h-screen" : ""}`}
    >
      {/* Video area — single or composed */}
      {viewMode === "composed" ? (
        // ── Composed view: 3×2 grid of all cameras ──
        // Container aspect ratio 8:3 ensures each 16:9 cell fills perfectly
        <div className="relative w-full bg-black" style={{ aspectRatio: "8/3" }}>
          <div
            className="h-full w-full"
            style={{
              display: "grid",
              gridTemplateAreas: '"lr front rr" "lp back rp"',
              gridTemplateColumns: "1fr 1fr 1fr",
              gridTemplateRows: "1fr 1fr",
              gap: "1px",
              background: "#0a0a0a",
            }}
          >
            {CAMERA_SLOTS.map(({ cam, area }) => {
              const file = getFileForCamera(cam);
              const encrypted = event.encrypted_videos?.[cam] ?? false;
              const hasCam = !!event.camera_videos[cam];
              const isMaster = cam === composedMaster;

              return (
                <div
                  key={cam}
                  style={{ gridArea: area }}
                  className="group/cell relative overflow-hidden bg-black"
                >
                  {!hasCam ? (
                    <div className="absolute inset-0 flex items-center justify-center text-zinc-800 text-[10px] uppercase tracking-widest">
                      {CAMERA_LABELS[cam]}
                    </div>
                  ) : encrypted ? (
                    <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-zinc-900/60 text-zinc-500 text-[10px]">
                      <LockIcon className="h-4 w-4" />
                      <span>Encrypted</span>
                    </div>
                  ) : file ? (
                    <video
                      ref={(el) => {
                        if (isMaster) mainVideoRef.current = el;
                        if (el) composedRefs.current.set(cam, el);
                        else composedRefs.current.delete(cam);
                      }}
                      src={streamUrlFn(file)}
                      className="h-full w-full object-cover"
                      playsInline
                      onTimeUpdate={isMaster ? onTimeUpdate : undefined}
                      onLoadedMetadata={(e) => {
                        if (isMaster) {
                          onLoadedMetadata();
                        } else {
                          // Seek follower to master position on load
                          const master = mainVideoRef.current;
                          if (master) e.currentTarget.currentTime = master.currentTime;
                        }
                      }}
                      onEnded={isMaster ? onEnded : undefined}
                      onPlay={isMaster ? () => setIsPlaying(true) : undefined}
                      onPause={isMaster ? () => setIsPlaying(false) : undefined}
                    />
                  ) : null}
                  {isMaster && (
                    <ArgusHUD
                      sei={currentSei}
                      visible={hudEnabled && seiFrames.length > 0}
                      scale={hudScale}
                      onScaleWheel={handleHudScaleWheel}
                      onScaleChange={updateHudScale}
                    />
                  )}
                  {hasCam && !encrypted && (
                    <div className="absolute bottom-1 left-1 rounded px-1 py-0.5 bg-black/50 text-[9px] font-medium text-white/50">
                      {CAMERA_LABELS[cam]}
                    </div>
                  )}
                  {hasCam && !encrypted && file && downloadUrlFn && (
                    <a
                      href={downloadUrlFn(file)}
                      download
                      className="absolute bottom-1 right-1 rounded p-0.5 bg-black/50 text-white/40 opacity-0 group-hover/cell:opacity-100 hover:!text-white transition-opacity"
                      title={`Download ${CAMERA_LABELS[cam]}`}
                      onClick={(e) => e.stopPropagation()}
                    >
                      <svg className="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M12 4v12m0 0-4-4m4 4 4-4" />
                      </svg>
                    </a>
                  )}
                </div>
              );
            })}
          </div>

          {/* Map overlay — bottom-right of composed grid */}
          {mapEnabled && seiFrames.length > 0 && (
            <div
              className="group absolute bottom-3 right-3 z-20"
              onWheel={handleMapScaleWheel}
              title={`Map size: ${mapScale.toFixed(2)}x (scroll or drag corner to resize)`}
            >
              <div className="relative" style={{ transform: `scale(${mapScale})`, transformOrigin: "bottom right" }}>
                <MapOverlay seiFrames={seiFrames} currentSei={currentSei} theme={mapTheme} />
                <CornerResize
                  scale={mapScale}
                  onChange={updateMapScale}
                  min={MAP_SCALE_MIN}
                  max={MAP_SCALE_MAX}
                  corner="tl"
                />
              </div>
            </div>
          )}
        </div>
      ) : (
        // ── Single camera view ──
        <div className="relative w-full bg-black" style={{ aspectRatio: "16/9" }}>
          {isEncrypted ? (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 px-6 text-center text-zinc-400">
              <LockIcon />
              <p className="text-sm">Encrypted by your Tesla account</p>
              <p className="max-w-sm text-xs text-zinc-500">
                This clip was encrypted on the drive (Tesla 2026.20+). View it at{" "}
                <a
                  href="https://dashcam.tesla.com"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-400 hover:underline"
                >
                  dashcam.tesla.com
                </a>{" "}
                with your Tesla account, or turn off Controls → Safety → “Encrypt Dashcam
                Recordings” for in-Argus playback.
              </p>
            </div>
          ) : streamSrc ? (
            <video
              ref={(el) => { mainVideoRef.current = el; }}
              src={streamSrc}
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
          <ArgusHUD
            sei={currentSei}
            visible={hudEnabled && seiFrames.length > 0}
            scale={hudScale}
            onScaleWheel={handleHudScaleWheel}
            onScaleChange={updateHudScale}
          />

          {/* Map overlay — bottom-right */}
          {mapEnabled && seiFrames.length > 0 && (
            <div
              className="group absolute bottom-3 right-3 z-20"
              onWheel={handleMapScaleWheel}
              title={`Map size: ${mapScale.toFixed(2)}x (scroll or drag corner to resize)`}
            >
              <div className="relative" style={{ transform: `scale(${mapScale})`, transformOrigin: "bottom right" }}>
                <MapOverlay seiFrames={seiFrames} currentSei={currentSei} theme={mapTheme} />
                <CornerResize
                  scale={mapScale}
                  onChange={updateMapScale}
                  min={MAP_SCALE_MIN}
                  max={MAP_SCALE_MAX}
                  corner="tl"
                />
              </div>
            </div>
          )}

          {/* Telemetry loading overlay */}
          {seiLoading && (
            <div className="absolute inset-0 z-30 flex items-center justify-center bg-black/70">
              <div className="flex flex-col items-center gap-3 rounded-lg bg-black/90 px-8 py-5 text-white backdrop-blur-sm">
                <div className="h-6 w-6 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                <span className="text-sm">{seiProgress}</span>
              </div>
            </div>
          )}

          {/* Overlay play button */}
          {!isPlaying && !isEncrypted && streamSrc && !seiLoading && (
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
      )}

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
            {/* Single video download */}
            {downloadUrlFn && activeFile && !isEncrypted && (
              <a
                href={downloadUrlFn(activeFile)}
                download
                className="rounded p-1.5 text-zinc-400 hover:text-white transition-colors"
                aria-label="Download current video"
                title="Download video"
              >
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v2a2 2 0 002 2h12a2 2 0 002-2v-2M12 4v12m0 0-4-4m4 4 4-4" />
                </svg>
              </a>
            )}

            {/* Event ZIP download (all camera videos + metadata) */}
            {downloadZipUrl && (
              <a
                href={downloadZipUrl}
                className="rounded p-1.5 text-zinc-400 hover:text-white transition-colors"
                aria-label="Download event ZIP"
                title="Download event ZIP (all cameras + metadata)"
              >
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5M9.75 11.625l2.25 2.25 2.25-2.25M12 13.875V6.75M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" />
                </svg>
              </a>
            )}

            {/* CSV telemetry export */}
            {hudEnabled && seiFrames.length > 0 && (
              <button
                onClick={() => downloadCsv(seiFrames, `${activeFile?.replace(/\.mp4$/i, "") ?? "telemetry"}.csv`)}
                className="rounded p-1.5 text-zinc-400 hover:text-white transition-colors"
                aria-label="Download telemetry CSV"
                title="Download telemetry CSV (parsed SEI data)"
              >
                {/* Data document with download arrow — distinct from the
                    ZIP archive box and the 3×2 camera grid icons. */}
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M19 5v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2Z" />
                  <path strokeLinecap="round" d="M8.5 8.5h7M8.5 11.5h7" />
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 14v4m0 0-2-2m2 2 2-2" />
                </svg>
              </button>
            )}

            {/* Composed / single view toggle */}
            <button
              onClick={() => setViewMode((m) => m === "composed" ? "single" : "composed")}
              className={`rounded p-1.5 transition-colors ${
                viewMode === "composed"
                  ? "text-[var(--color-accent)]"
                  : "text-zinc-400 hover:text-white"
              }`}
              aria-label="Toggle composed view"
              title="All cameras"
            >
              {/* 3×2 grid icon */}
              <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
                <rect x="2"  y="2"  width="6" height="9" rx="1" />
                <rect x="9"  y="2"  width="6" height="9" rx="1" />
                <rect x="16" y="2"  width="6" height="9" rx="1" />
                <rect x="2"  y="13" width="6" height="9" rx="1" />
                <rect x="9"  y="13" width="6" height="9" rx="1" />
                <rect x="16" y="13" width="6" height="9" rx="1" />
              </svg>
            </button>

            {/* Map toggle */}
            <button
              onClick={toggleMap}
              className={`rounded p-1.5 transition-colors ${
                mapEnabled ? "text-[var(--color-accent)]" : "text-zinc-400 hover:text-white"
              }`}
              aria-label="Toggle map overlay"
              title="Map"
            >
              <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M15 10.5a3 3 0 11-6 0 3 3 0 016 0z" />
                <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 10.5c0 7.142-7.5 11.25-7.5 11.25S4.5 17.642 4.5 10.5a7.5 7.5 0 1115 0z" />
              </svg>
            </button>

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

      {/* Camera thumbnails — hidden in composed mode (all cameras already visible) */}
      {viewMode === "single" && cameras.length > 1 && (
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
