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
import { useToast } from "./Toast";
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
  /** Server-side trim of one camera file. Omitted where the backend can't trim. */
  exportUrlFn?: (cameraFile: string, start: number, end: number) => string;
  /** Poster for the camera strip; without it the strip shows placeholders. */
  thumbnailUrlFn?: (camera: string) => string;
  downloadZipUrl?: string;
  /** Camera that triggered the event, so it opens on the angle that matters. */
  initialCamera?: CameraName;
  /** Deep-linked position (?clip=&t=), so a URL carries "clip 4, 0:37". */
  initialClipIndex?: number;
  initialTime?: number;
  onPositionChange?: (clipIndex: number, time: number) => void;
  onDelete?: () => void;
}

const SEEK_STEP = 5;
// Tesla's nominal clip length — the assumed duration of a clip that hasn't
// loaded yet, replaced by the real one as soon as that clip reports metadata.
const CLIP_SECONDS = 60;
// Tesla cameras record ~36fps — close enough to step to the contact frame.
const FRAME_STEP = 1 / 36;
const RATES = [0.5, 1, 1.5, 2];

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
  exportUrlFn,
  thumbnailUrlFn,
  downloadZipUrl,
  initialCamera,
  initialClipIndex,
  initialTime,
  onPositionChange,
  onDelete,
}: DashcamPlayerProps) {
  const { showToast } = useToast();
  const cameras = useMemo<CameraName[]>(
    () =>
      (Object.keys(event.camera_videos) as CameraName[]).sort(
        (a, b) => cameraOrder(a) - cameraOrder(b),
      ),
    [event.camera_videos],
  );

  const clips = useMemo(() => event.clips ?? [], [event.clips]);

  const [activeCamera, setActiveCamera] = useState<CameraName>(
    initialCamera && cameras.includes(initialCamera)
      ? initialCamera
      : cameras.includes("front")
        ? "front"
        : cameras[0],
  );
  // ?clip= comes from the URL, so floor and clamp it rather than trusting it:
  // a fractional index reads clips[] as undefined and lands on the wrong clip.
  const [clipIndex, setClipIndex] = useState(() => {
    const want = Math.floor(initialClipIndex ?? event.starting_clip_index ?? 0);
    if (!Number.isFinite(want)) return 0;
    return Math.max(0, Math.min(want, Math.max(0, (event.clips?.length ?? 1) - 1)));
  });
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  // Real per-clip durations, learned from each clip as it loads. Tesla's first
  // clip of an event is frequently short, and probing every clip up front would
  // be a request storm on the Pi.
  const [clipDurations, setClipDurations] = useState<Record<number, number>>({});
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [pseudoFs, setPseudoFs] = useState(false);
  // Six simultaneous streams is more than the Pi's AP can serve; the grid is
  // opt-in and the choice is remembered, same as the HUD/map toggles.
  const [viewMode, setViewMode] = useState<"single" | "composed">("single");
  // The 3×2 grid is unreadable on a portrait phone, so it stacks 2×3 there.
  const [wideGrid, setWideGrid] = useState(true);
  const [rate, setRate] = useState(1);
  const [streamError, setStreamError] = useState(false);
  const [buffering, setBuffering] = useState(false);
  const [zipBusy, setZipBusy] = useState(false);
  const [markIn, setMarkIn] = useState<number | null>(null);
  const [markOut, setMarkOut] = useState<number | null>(null);
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
  const seiAbortRef = useRef<AbortController | null>(null);
  const seiLoadingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const zipTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Populated only in composed mode; empty in single mode (makes sync calls no-ops)
  const composedRefs = useRef<Map<CameraName, HTMLVideoElement>>(new Map());
  // Set when onEnded auto-advances the clip so the next clip resumes on load
  const pendingAutoplayRef = useRef(false);
  // Offset to seek to once the newly-selected clip reports its metadata. A
  // non-finite ?t= would poison currentTime, so it never becomes a seek.
  // Absent a deep link this opens on the trigger frame: nobody opens a Sentry
  // event to watch the empty minute before it. An explicit ?clip=/?t= wins —
  // that URL was built by someone who already chose a moment.
  const pendingSeekRef = useRef<number | null>(
    typeof initialTime === "number" && Number.isFinite(initialTime)
      ? initialTime
      : initialClipIndex === undefined
        ? triggerOffsetInClip(event, clipIndex)
        : null,
  );
  // Playback rate has to be re-applied per clip; a ref keeps handlers stable.
  const rateRef = useRef(1);

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

  // ── Virtual timeline ──
  // A multi-clip event is one continuous incident, so the seek bar spans the
  // whole event rather than the current 60s file.
  const multiClip = clips.length > 1;
  // Where a clip begins on the event-wide timeline. Clips that haven't been
  // played yet fall back to Tesla's nominal length, so the mapping self-corrects
  // as their real durations arrive.
  const clipStart = useCallback(
    (i: number) => {
      let start = 0;
      for (let k = 0; k < i; k++) start += clipDurations[k] ?? CLIP_SECONDS;
      return start;
    },
    [clipDurations],
  );
  const totalDuration = multiClip
    ? clipStart(clips.length - 1) + (clipDurations[clips.length - 1] ?? CLIP_SECONDS)
    : duration;
  const virtualTime = multiClip ? clipStart(clipIndex) + currentTime : currentTime;

  // Report the position so the page can put it in the URL — a reload, or a link
  // sent to another phone, lands on the same frame. Nothing is reported before
  // the clip has loaded and any deferred seek has landed: t=0 published then
  // would overwrite the very deep link that opened the page.
  const lastReportedClipRef = useRef<number | null>(null);
  const reportPosition = useCallback(() => {
    const v = mainVideoRef.current;
    if (!v || pendingSeekRef.current !== null || !isFinite(v.currentTime)) return;
    lastReportedClipRef.current = clipIndex;
    onPositionChange?.(clipIndex, v.currentTime);
  }, [clipIndex, onPositionChange]);

  // Restore HUD + map toggles and the view mode from localStorage. Scales are
  // handled by useViewerScale (server-persisted with localStorage cache).
  useEffect(() => {
    try {
      const hudOn = localStorage.getItem("seiOverlayEnabled") !== "false";
      const mapOn = localStorage.getItem("mapOverlayEnabled") !== "false";
      setHudEnabled(hudOn);
      setMapEnabled(mapOn);
      if (localStorage.getItem("viewMode") === "composed") setViewMode("composed");
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
    setStreamError(false);
    setBuffering(false);
    // In/out markers address a single camera file, so they can't survive it.
    setMarkIn(null);
    setMarkOut(null);

    if ((hudEnabled || mapEnabled) && activeFile && !isEncrypted) {
      loadTelemetry(activeFile);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeFile]);

  // Track the sm breakpoint so the composed grid can restack for portrait phones.
  useEffect(() => {
    const mql = window.matchMedia("(min-width: 640px)");
    const sync = () => setWideGrid(mql.matches);
    sync();
    mql.addEventListener("change", sync);
    return () => mql.removeEventListener("change", sync);
  }, []);

  // Main video event handlers
  const onTimeUpdate = useCallback(() => {
    const v = mainVideoRef.current;
    if (!v) return;
    setCurrentTime(v.currentTime);

    // Keep composed follower cameras in sync
    composedRefs.current.forEach((cv) => {
      if (cv !== v && Math.abs(cv.currentTime - v.currentTime) > 0.5) {
        cv.currentTime = v.currentTime;
      }
    });

    if (seiFrames.length > 0) {
      setCurrentSei(findSeiAtTime(seiFrames, v.currentTime));
    }
  }, [seiFrames]);

  const onLoadedMetadata = useCallback(() => {
    const v = mainVideoRef.current;
    if (!v) return;
    setDuration(v.duration);
    v.playbackRate = rateRef.current;
    if (isFinite(v.duration)) {
      const known = v.duration;
      setClipDurations((prev) =>
        prev[clipIndex] === known ? prev : { ...prev, [clipIndex]: known },
      );
    }

    // Land on the position a scrub (or a ?t= deep link) asked for
    if (pendingSeekRef.current !== null) {
      const target = Math.min(pendingSeekRef.current, v.duration || 0);
      pendingSeekRef.current = null;
      v.currentTime = target;
      setCurrentTime(target);
      composedRefs.current.forEach((cv) => { if (cv !== v) cv.currentTime = target; });
    }

    // Resume playback after an auto-advance so multi-clip events play through
    if (pendingAutoplayRef.current) {
      pendingAutoplayRef.current = false;
      v.play()
        .then(() => {
          setIsPlaying(true);
          composedRefs.current.forEach((cv) => {
            if (cv !== v) { cv.currentTime = v.currentTime; cv.play().catch(() => {}); }
          });
        })
        .catch(() => {});
    }

    // A clip swap only has a real position once the new file has loaded.
    if (lastReportedClipRef.current !== clipIndex) reportPosition();
  }, [clipIndex, clips.length, reportPosition]);

  const onEnded = useCallback(() => {
    setIsPlaying(false);
    if (clipIndex < clips.length - 1) {
      pendingAutoplayRef.current = true;
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
    setCurrentTime(time);
    composedRefs.current.forEach((cv) => { if (cv !== v) cv.currentTime = time; });
  }, []);

  // Seek anywhere in the event: crossing a clip boundary swaps the file and
  // defers the seek until the new clip reports metadata.
  const seekVirtual = useCallback((t: number) => {
    if (!multiClip) { seekTo(Math.max(0, Math.min(t, duration || 0))); return; }
    const clamped = Math.max(0, Math.min(t, totalDuration));
    let idx = 0;
    while (idx < clips.length - 1 && clamped >= clipStart(idx + 1)) idx++;
    const offset = clamped - clipStart(idx);
    if (idx === clipIndex) { seekTo(offset); return; }
    pendingSeekRef.current = offset;
    setClipIndex(idx);
  }, [multiClip, seekTo, duration, totalDuration, clips.length, clipIndex, clipStart]);

  const applyRate = useCallback((next: number) => {
    rateRef.current = next;
    setRate(next);
    const v = mainVideoRef.current;
    if (v) v.playbackRate = next;
    composedRefs.current.forEach((cv) => { cv.playbackRate = next; });
  }, []);

  const stepFrame = useCallback((dir: number) => {
    const v = mainVideoRef.current;
    if (!v) return;
    v.pause();
    setIsPlaying(false);
    composedRefs.current.forEach((cv) => { if (cv !== v) cv.pause(); });
    seekTo(Math.max(0, Math.min(v.currentTime + dir * FRAME_STEP, v.duration || 0)));
  }, [seekTo]);

  const toggleFullscreen = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
      return;
    }
    if (el.requestFullscreen) {
      el.requestFullscreen().catch(() => {});
      return;
    }
    // iOS Safari has no Element.requestFullscreen: only a <video> can go
    // fullscreen, and only one — so the grid falls back to filling the viewport.
    const v = mainVideoRef.current as
      | (HTMLVideoElement & { webkitEnterFullscreen?: () => void })
      | null;
    if (viewMode === "single" && v?.webkitEnterFullscreen) {
      v.webkitEnterFullscreen();
      return;
    }
    setPseudoFs((p) => !p);
  }, [viewMode]);

  // Fullscreen change detection
  useEffect(() => {
    const onFsChange = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", onFsChange);
    return () => document.removeEventListener("fullscreenchange", onFsChange);
  }, []);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      // Only fire from the page background — otherwise Space "plays" instead of
      // pressing the Delete button the user just focused.
      const target = e.target as HTMLElement | null;
      if (target?.closest?.('button, a, select, input, textarea, [role="slider"]')) {
        return;
      }

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
        case ",":
          e.preventDefault();
          stepFrame(-1);
          break;
        case ".":
          e.preventDefault();
          stepFrame(1);
          break;
        case "Escape":
          setPseudoFs(false);
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
  }, [togglePlay, seek, stepFrame, toggleFullscreen]);

  // ── Seek bar scrubbing ──
  // Pointer Events (not click) so finding the moment of impact is a drag, not a
  // 6px stab; pointer capture keeps the drag alive outside the bar.
  const scrubbingRef = useRef(false);
  const scrubRafRef = useRef<number | null>(null);
  const scrubXRef = useRef(0);

  const scrubTo = useCallback((clientX: number) => {
    const bar = seekBarRef.current;
    if (!bar || !totalDuration) return;
    const rect = bar.getBoundingClientRect();
    const pct = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    seekVirtual(pct * totalDuration);
  }, [totalDuration, seekVirtual]);

  const onSeekPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    scrubbingRef.current = true;
    e.currentTarget.setPointerCapture(e.pointerId);
    scrubTo(e.clientX);
  }, [scrubTo]);

  const onSeekPointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!scrubbingRef.current) return;
    // One seek per frame: a raw pointermove stream would thrash the Pi with
    // range requests for every pixel of the drag.
    scrubXRef.current = e.clientX;
    if (scrubRafRef.current !== null) return;
    scrubRafRef.current = requestAnimationFrame(() => {
      scrubRafRef.current = null;
      scrubTo(scrubXRef.current);
    });
  }, [scrubTo]);

  const onSeekPointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    scrubbingRef.current = false;
    e.currentTarget.releasePointerCapture(e.pointerId);
    if (scrubRafRef.current !== null) {
      cancelAnimationFrame(scrubRafRef.current);
      scrubRafRef.current = null;
    }
    // Once per drag, not once per frame: history.replaceState is rate-limited.
    reportPosition();
  }, [reportPosition]);

  const onSeekKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    const step = e.key === "ArrowLeft" ? -SEEK_STEP : e.key === "ArrowRight" ? SEEK_STEP : 0;
    if (!step) return;
    e.preventDefault();
    seekVirtual(virtualTime + step);
  }, [seekVirtual, virtualTime]);

  const retryStream = useCallback(() => {
    const v = mainVideoRef.current;
    setStreamError(false);
    if (!v) return;
    const src = v.src;
    v.src = "";
    v.src = src;
    v.load();
  }, []);

  // ── Telemetry fetch ──

  const abortSei = useCallback(() => {
    if (seiAbortRef.current) {
      seiAbortRef.current.abort();
      seiAbortRef.current = null;
    }
    if (seiLoadingTimerRef.current) {
      clearTimeout(seiLoadingTimerRef.current);
      seiLoadingTimerRef.current = null;
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
      seiLoadingTimerRef.current = setTimeout(() => setSeiLoading(false), 500);
    } catch (err: unknown) {
      if (err instanceof DOMException && err.name === "AbortError") {
        setSeiLoading(false);
        return;
      }
      setSeiProgress(`Failed: ${err instanceof Error ? err.message : "unknown error"}`);
      seiLoadingTimerRef.current = setTimeout(() => setSeiLoading(false), 2000);
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

  // Same passive-listener problem as the map below: the HUD is wrapped in a
  // plain div so the wheel event can be caught natively as it bubbles.
  const hudScaleRef = useRef(hudScale);
  hudScaleRef.current = hudScale;
  const hudWheelRef = useCallback((el: HTMLDivElement | null) => {
    if (!el) return;
    const handler = (e: WheelEvent) => {
      e.preventDefault();
      const step = e.deltaY < 0 ? 0.1 : -0.1;
      updateHudScale(hudScaleRef.current + step);
    };
    el.addEventListener("wheel", handler, { passive: false });
    return () => el.removeEventListener("wheel", handler);
  }, [updateHudScale]);

  // React registers root wheel listeners as passive, so e.preventDefault() in a
  // React onWheel handler is a no-op. Attach a native non-passive listener via a
  // ref-cleanup callback so wheel-resizing the map doesn't also scroll the page.
  const mapScaleRef = useRef(mapScale);
  mapScaleRef.current = mapScale;
  const mapWheelRef = useCallback((el: HTMLDivElement | null) => {
    if (!el) return;
    const handler = (e: WheelEvent) => {
      e.preventDefault();
      const step = e.deltaY < 0 ? 0.1 : -0.1;
      updateMapScale(mapScaleRef.current + step);
    };
    el.addEventListener("wheel", handler, { passive: false });
    return () => el.removeEventListener("wheel", handler);
  }, [updateMapScale]);

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

  // Abort any in-flight fetch and drop pending timers/frames on unmount
  useEffect(() => {
    return () => {
      abortSei();
      if (zipTimerRef.current) clearTimeout(zipTimerRef.current);
      if (scrubRafRef.current !== null) cancelAnimationFrame(scrubRafRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Clear composedRefs when leaving composed mode
  useEffect(() => {
    if (viewMode === "single") {
      composedRefs.current.clear();
    }
  }, [viewMode]);

  const toggleViewMode = useCallback(() => {
    setViewMode((m) => {
      const next = m === "composed" ? "single" : "composed";
      try { localStorage.setItem("viewMode", next); } catch {}
      return next;
    });
  }, []);

  const onVideoPause = useCallback(() => {
    setIsPlaying(false);
    reportPosition();
  }, [reportPosition]);

  const handleZipClick = useCallback(() => {
    showToast("Preparing ZIP — large events take a minute");
    setZipBusy(true);
    // The Pi zips serially; a second click just queues another slow job.
    zipTimerRef.current = setTimeout(() => setZipBusy(false), 10000);
  }, [showToast]);

  // Telemetry progress as a corner pill — it used to be a full-bleed overlay
  // that hid a video that was ready to play.
  const seiPill = seiLoading ? (
    <div className="absolute bottom-1 left-1 z-20 rounded px-1.5 py-0.5 bg-black/50 text-[10px] font-medium text-white/70">
      {seiProgress}
    </div>
  ) : null;

  const bufferSpinner = buffering && !streamError ? (
    <div className="absolute right-2 top-2 z-20 h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
  ) : null;

  const errorOverlay = streamError ? (
    <div className="absolute inset-0 z-30 flex flex-col items-center justify-center gap-3 bg-black/85 px-6 text-center">
      <p className="text-sm text-zinc-300">This clip failed to load</p>
      <button
        onClick={retryStream}
        className="rounded-sm bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90"
      >
        Retry
      </button>
    </div>
  ) : null;

  return (
    <div
      ref={containerRef}
      className={`flex flex-col w-full bg-black ${isFullscreen ? "h-screen" : ""} ${pseudoFs ? "fixed inset-0 z-50" : ""}`}
    >
      {/* Video area — single or composed */}
      {viewMode === "composed" ? (
        // ── Composed view: all cameras at once ──
        // 3×2 at ≥sm (8:3 makes each 16:9 cell fill exactly); 2×3 below, where
        // a six-across grid is too small to read.
        <div
          className="relative w-full bg-black"
          style={{ aspectRatio: wideGrid ? "8/3" : "32/27" }}
        >
          <div
            className="h-full w-full"
            style={{
              display: "grid",
              gridTemplateAreas: wideGrid
                ? '"lr front rr" "lp back rp"'
                : '"front back" "lr rr" "lp rp"',
              gridTemplateColumns: wideGrid ? "1fr 1fr 1fr" : "1fr 1fr",
              gridTemplateRows: wideGrid ? "1fr 1fr" : "1fr 1fr 1fr",
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
                      onPause={isMaster ? onVideoPause : undefined}
                      onError={isMaster ? () => setStreamError(true) : undefined}
                      onWaiting={isMaster ? () => setBuffering(true) : undefined}
                      onPlaying={isMaster ? () => setBuffering(false) : undefined}
                    />
                  ) : null}
                  {isMaster && (
                    <div ref={hudWheelRef}>
                      <ArgusHUD
                        sei={currentSei}
                        visible={hudEnabled && seiFrames.length > 0}
                        scale={hudScale}
                        onScaleChange={updateHudScale}
                      />
                    </div>
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
                      className="absolute bottom-1 right-1 rounded p-0.5 bg-black/50 text-white/40 opacity-0 group-hover/cell:opacity-100 hover:!text-white transition-opacity [@media(pointer:coarse)]:opacity-100"
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
              ref={mapWheelRef}
              className="group absolute bottom-3 right-3 z-20"
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

          {seiPill}
          {bufferSpinner}
          {errorOverlay}
        </div>
      ) : (
        // ── Single camera view ──
        <div className="relative w-full bg-black" style={{ aspectRatio: "16/9" }}>
          {isEncrypted ? (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 px-6 text-center text-zinc-400">
              <LockIcon />
              <p className="text-sm">Encrypted or unreadable</p>
              <p className="max-w-sm text-xs text-zinc-500">
                Either this clip was encrypted on the drive (Tesla 2026.20+) — view it at{" "}
                <a
                  href="https://dashcam.tesla.com"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-400 hover:underline"
                >
                  dashcam.tesla.com
                </a>{" "}
                with your Tesla account, or turn off Controls → Safety → “Encrypt Dashcam
                Recordings” for in-Argus playback — or the file is corrupt, which
                looks identical here and is common after an unclean power-off. Try
                Analytics → Repair.
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
              onPause={onVideoPause}
              onError={() => setStreamError(true)}
              onWaiting={() => setBuffering(true)}
              onPlaying={() => setBuffering(false)}
              playsInline
            />
          ) : (
            <div className="absolute inset-0 flex items-center justify-center text-zinc-500 text-sm">
              No video available
            </div>
          )}

          {/* Tesla HUD Overlay */}
          <div ref={hudWheelRef}>
            <ArgusHUD
              sei={currentSei}
              visible={hudEnabled && seiFrames.length > 0}
              scale={hudScale}
              onScaleChange={updateHudScale}
            />
          </div>

          {/* Map overlay — bottom-right */}
          {mapEnabled && seiFrames.length > 0 && (
            <div
              ref={mapWheelRef}
              className="group absolute bottom-3 right-3 z-20"
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

          {seiPill}
          {bufferSpinner}
          {errorOverlay}

          {/* Overlay play button */}
          {!isPlaying && !isEncrypted && streamSrc && !streamError && (
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
        {/* Seek bar — spans the whole event, not just the current clip. The
            -my-2/py-2 wrapper is the touch target; the bar itself stays thin. */}
        <div
          className="group relative -my-2 w-full cursor-pointer touch-none py-2"
          role="slider"
          tabIndex={0}
          aria-label="Seek"
          aria-valuemin={0}
          aria-valuemax={Math.round(totalDuration) || 0}
          aria-valuenow={Math.round(virtualTime)}
          aria-valuetext={`${formatTime(virtualTime)} of ${formatTime(totalDuration)}`}
          onPointerDown={onSeekPointerDown}
          onPointerMove={onSeekPointerMove}
          onPointerUp={onSeekPointerUp}
          onPointerCancel={onSeekPointerUp}
          onKeyDown={onSeekKeyDown}
        >
          <div
            ref={seekBarRef}
            className="relative h-1.5 w-full rounded-full bg-zinc-700 transition-all group-hover:h-2.5"
          >
            <div
              className="h-full rounded-full bg-[var(--color-accent)]"
              style={{ width: `${pct(virtualTime, totalDuration)}%` }}
            />

            {/* Clip boundaries */}
            {multiClip &&
              clips.slice(1).map((_, i) => (
                <div
                  key={i}
                  className="absolute inset-y-0 w-px bg-black/60"
                  style={{ left: `${pct(clipStart(i + 1), totalDuration)}%` }}
                />
              ))}

            {/* The trigger moment — the only frame most people came for. An
                absent offset arrives as 0, which would mark every event at 0:00. */}
            {event.trigger_offset_sec !== undefined &&
              event.trigger_offset_sec > 0 &&
              totalDuration > 0 && (
                <div
                  className="absolute -inset-y-1 w-0.5 bg-[var(--color-danger)]"
                  style={{ left: `${pct(event.trigger_offset_sec, totalDuration)}%` }}
                  title="Trigger"
                />
              )}

            {/* Export in/out markers */}
            {markIn !== null && (
              <div
                className="absolute -inset-y-1 w-0.5 bg-[var(--color-success)]"
                style={{ left: `${pct((multiClip ? clipStart(clipIndex) : 0) + markIn, totalDuration)}%` }}
              />
            )}
            {markOut !== null && (
              <div
                className="absolute -inset-y-1 w-0.5 bg-[var(--color-success)]"
                style={{ left: `${pct((multiClip ? clipStart(clipIndex) : 0) + markOut, totalDuration)}%` }}
              />
            )}

            {/* Handle — always visible once there is something to scrub */}
            {totalDuration > 0 && (
              <div
                className="absolute top-1/2 h-3.5 w-3.5 -translate-y-1/2 rounded-full bg-[var(--color-accent)] shadow"
                style={{ left: `calc(${pct(virtualTime, totalDuration)}% - 7px)` }}
              />
            )}
          </div>
        </div>

        {/* Button row */}
        <div className="flex flex-wrap items-center justify-between gap-y-2">
          <div className="flex flex-wrap items-center gap-2">
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

            {/* Time display — event-wide, matching the seek bar */}
            <span className="ml-2 text-xs tabular-nums text-zinc-400">
              {formatTime(virtualTime)} / {formatTime(totalDuration)}
            </span>

            {clips.length > 1 && (
              <span className="text-xs text-zinc-500 ml-1">
                Clip {clipIndex + 1}/{clips.length}
              </span>
            )}

            {/* Playback speed — reading a plate needs slow, scanning needs fast */}
            <button
              onClick={() => applyRate(RATES[(RATES.indexOf(rate) + 1) % RATES.length])}
              className="rounded px-1.5 py-1 text-xs font-medium tabular-nums text-zinc-300 hover:text-white transition-colors"
              title="Playback speed"
            >
              {rate}x
            </button>

            {/* Frame stepping — the contact frame is between two 5s jumps */}
            {!isPlaying && (
              <>
                <button
                  onClick={() => stepFrame(-1)}
                  className="rounded px-1.5 py-1 text-xs font-medium text-zinc-300 hover:text-white transition-colors"
                  aria-label="Previous frame"
                  title="Previous frame (,)"
                >
                  ◄|
                </button>
                <button
                  onClick={() => stepFrame(1)}
                  className="rounded px-1.5 py-1 text-xs font-medium text-zinc-300 hover:text-white transition-colors"
                  aria-label="Next frame"
                  title="Next frame (.)"
                >
                  |►
                </button>
              </>
            )}

            {/* Trim & export — send someone the 30 seconds that matter, not 30 MB */}
            {exportUrlFn && activeFile && !isEncrypted && (
              <>
                <button
                  onClick={() => setMarkIn(currentTime)}
                  className="rounded px-1.5 py-1 text-xs font-medium text-zinc-300 hover:text-white transition-colors"
                  title="Mark clip start"
                >
                  In
                </button>
                <button
                  onClick={() => setMarkOut(currentTime)}
                  className="rounded px-1.5 py-1 text-xs font-medium text-zinc-300 hover:text-white transition-colors"
                  title="Mark clip end"
                >
                  Out
                </button>
                {markIn !== null && markOut !== null && markOut > markIn && (
                  <a
                    href={exportUrlFn(activeFile, markIn, markOut)}
                    download
                    className="rounded bg-[var(--color-accent)] px-2 py-1 text-xs font-medium text-white hover:opacity-90"
                    title={`Export ${formatTime(markIn)}–${formatTime(markOut)}`}
                  >
                    Export clip
                  </a>
                )}
              </>
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
                onClick={handleZipClick}
                className={`rounded p-1.5 text-zinc-400 hover:text-white transition-colors ${zipBusy ? "pointer-events-none opacity-50" : ""}`}
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
              onClick={toggleViewMode}
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
              const poster = file && thumbnailUrlFn ? thumbnailUrlFn(cam) : null;

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
                  ) : poster ? (
                    // A cached JPEG per camera, not a second video stream — six
                    // preload="metadata" videos was the Pi's whole upload budget.
                    <img
                      src={poster}
                      alt=""
                      className="h-full w-full object-cover"
                      loading="lazy"
                    />
                  ) : (
                    // Sessions have no per-camera posters; name the camera so the
                    // strip still reads as a switcher instead of as broken.
                    <div className="absolute inset-0 flex items-center justify-center bg-zinc-800 px-1 text-center text-[11px] font-medium text-zinc-300">
                      {CAMERA_LABELS[cam] ?? cam}
                    </div>
                  )}
                  {(encrypted || poster) && (
                    <span className="absolute bottom-0 inset-x-0 bg-gradient-to-t from-black/80 to-transparent px-1.5 py-1 text-[10px] font-medium text-white text-center">
                      {CAMERA_LABELS[cam] ?? cam}
                    </span>
                  )}
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

// Tesla names each clip file for its own wall-clock start, "2024-01-02_15-04-05".
function clipStartSec(ts: string): number {
  return Date.parse(`${ts.slice(0, 10)}T${ts.slice(11).replace(/-/g, ":")}`) / 1000;
}

// trigger_offset_sec is measured from the start of clips[0], but the player
// loads one clip file at a time — so the opening clip's own start has to come
// off it. That start is read from the filenames rather than assumed to be
// 60s per clip: a Sentry event skips idle minutes, and a nominal offset past
// the end of the file would clamp to the end of the clip and miss the moment.
// null means "no usable trigger", which leaves the clip opening at 0 as before.
function triggerOffsetInClip(event: VideoEvent, clipIndex: number): number | null {
  const offset = event.trigger_offset_sec;
  if (!offset || !isFinite(offset) || offset <= 0) return null;
  const clips = event.clips ?? [];
  if (clipIndex === 0 || !clips[clipIndex] || !clips[0]) return offset;
  const elapsed = clipStartSec(clips[clipIndex]) - clipStartSec(clips[0]);
  if (!isFinite(elapsed)) return null;
  return Math.max(0, offset - elapsed);
}

function pct(value: number, total: number): number {
  if (!total || !isFinite(total)) return 0;
  return Math.max(0, Math.min(100, (value / total) * 100));
}

function formatTime(seconds: number): string {
  if (!isFinite(seconds) || seconds < 0) return "0:00";
  const m = Math.floor(seconds / 60);
  const s = Math.floor(seconds % 60);
  return `${m}:${s.toString().padStart(2, "0")}`;
}
