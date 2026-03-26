"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

/* ------------------------------------------------------------------ */
/*  Types & defaults                                                   */
/* ------------------------------------------------------------------ */

interface AudioTrimmerProps {
  onExport: (blob: Blob, filename: string) => void;
  maxSize?: number;
  maxDuration?: number;
  minDuration?: number;
  speedMin?: number;
  speedMax?: number;
  speedStep?: number;
}

const SAMPLE_RATE = 44100;
const WAVEFORM_POINTS = 2000;
const DEFAULT_MAX_SIZE = 1_000_000; // 1 MB
const DEFAULT_MAX_DURATION = 10; // 10s
const DEFAULT_MIN_DURATION = 0.1;

/* ------------------------------------------------------------------ */
/*  WSOLA time-stretch (pitch-preserving)                              */
/* ------------------------------------------------------------------ */

function wsolaStretch(samples: Float32Array, factor: number): Float32Array {
  if (Math.abs(factor - 1) < 0.001) return samples;

  const outLen = Math.round(samples.length / factor);
  const out = new Float32Array(outLen);

  const windowSize = 1024;
  const halfWin = windowSize >> 1;
  const hopAnalysis = 256;
  const hopSynthesis = Math.round(hopAnalysis / factor);
  const searchRange = 64;

  let readPos = 0;
  let writePos = 0;

  // Hann window
  const win = new Float32Array(windowSize);
  for (let i = 0; i < windowSize; i++) {
    win[i] = 0.5 * (1 - Math.cos((2 * Math.PI * i) / (windowSize - 1)));
  }

  while (writePos + windowSize < outLen && readPos + windowSize < samples.length) {
    // Find best overlap in search range
    let bestOffset = 0;
    let bestCorr = -Infinity;

    for (let d = -searchRange; d <= searchRange; d++) {
      const pos = readPos + d;
      if (pos < 0 || pos + windowSize >= samples.length) continue;

      let corr = 0;
      // Correlate on a sub-sampled basis for speed
      for (let i = 0; i < windowSize; i += 4) {
        corr += samples[pos + i] * (writePos + i < outLen ? out[writePos + i] || 0 : 0);
      }
      if (corr > bestCorr) {
        bestCorr = corr;
        bestOffset = d;
      }
    }

    const srcPos = readPos + bestOffset;
    for (let i = 0; i < windowSize && writePos + i < outLen && srcPos + i < samples.length; i++) {
      out[writePos + i] += samples[srcPos + i] * win[i];
    }

    readPos += hopAnalysis;
    writePos += hopSynthesis;
  }

  // Normalize to prevent clipping
  let peak = 0;
  for (let i = 0; i < out.length; i++) {
    const abs = Math.abs(out[i]);
    if (abs > peak) peak = abs;
  }
  if (peak > 1) {
    const scale = 1 / peak;
    for (let i = 0; i < out.length; i++) out[i] *= scale;
  }

  return out;
}

/* ------------------------------------------------------------------ */
/*  RMS→LUFS-approximate normalization                                 */
/* ------------------------------------------------------------------ */

function normalizeLUFS(samples: Float32Array, targetLUFS: number): Float32Array {
  let sumSq = 0;
  for (let i = 0; i < samples.length; i++) sumSq += samples[i] * samples[i];
  const rms = Math.sqrt(sumSq / samples.length);

  if (rms < 1e-8) return samples;

  // RMS to LUFS approximation: LUFS ≈ 20*log10(rms) - 0.691
  const currentLUFS = 20 * Math.log10(rms) - 0.691;
  const gainDB = targetLUFS - currentLUFS;
  const gainLinear = Math.pow(10, gainDB / 20);

  // Limit gain to prevent insane amplification
  const clampedGain = Math.min(gainLinear, 20);
  const result = new Float32Array(samples.length);
  for (let i = 0; i < samples.length; i++) {
    result[i] = Math.max(-1, Math.min(1, samples[i] * clampedGain));
  }
  return result;
}

/* ------------------------------------------------------------------ */
/*  WAV encoder                                                        */
/* ------------------------------------------------------------------ */

function encodeWAV(samples: Float32Array, sampleRate: number): Blob {
  const numChannels = 1;
  const bitsPerSample = 16;
  const byteRate = sampleRate * numChannels * (bitsPerSample / 8);
  const blockAlign = numChannels * (bitsPerSample / 8);
  const dataSize = samples.length * (bitsPerSample / 8);

  const buffer = new ArrayBuffer(44 + dataSize);
  const view = new DataView(buffer);

  const writeString = (offset: number, s: string) => {
    for (let i = 0; i < s.length; i++) view.setUint8(offset + i, s.charCodeAt(i));
  };

  writeString(0, "RIFF");
  view.setUint32(4, 36 + dataSize, true);
  writeString(8, "WAVE");
  writeString(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, numChannels, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, byteRate, true);
  view.setUint16(32, blockAlign, true);
  view.setUint16(34, bitsPerSample, true);
  writeString(36, "data");
  view.setUint32(40, dataSize, true);

  let offset = 44;
  for (let i = 0; i < samples.length; i++) {
    const clamped = Math.max(-1, Math.min(1, samples[i]));
    const val = clamped < 0 ? clamped * 0x8000 : clamped * 0x7fff;
    view.setInt16(offset, val, true);
    offset += 2;
  }

  return new Blob([buffer], { type: "audio/wav" });
}

/* ------------------------------------------------------------------ */
/*  Peak waveform extraction                                           */
/* ------------------------------------------------------------------ */

function extractPeaks(buffer: AudioBuffer, points: number): Float32Array {
  const channel = buffer.getChannelData(0);
  const peaks = new Float32Array(points);
  const step = channel.length / points;

  for (let i = 0; i < points; i++) {
    const start = Math.floor(i * step);
    const end = Math.min(Math.floor((i + 1) * step), channel.length);
    let max = 0;
    for (let j = start; j < end; j++) {
      const abs = Math.abs(channel[j]);
      if (abs > max) max = abs;
    }
    peaks[i] = max;
  }
  return peaks;
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

export default function AudioTrimmer({
  onExport,
  maxSize = DEFAULT_MAX_SIZE,
  maxDuration = DEFAULT_MAX_DURATION,
  minDuration = DEFAULT_MIN_DURATION,
  speedMin = 0.5,
  speedMax = 2.0,
  speedStep = 0.05,
}: AudioTrimmerProps) {
  // Audio state
  const [audioBuffer, setAudioBuffer] = useState<AudioBuffer | null>(null);
  const [fileName, setFileName] = useState("audio");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Trim region (0..1 fraction of total)
  const [trimStart, setTrimStart] = useState(0);
  const [trimEnd, setTrimEnd] = useState(1);

  // Controls
  const [speed, setSpeed] = useState(1);
  const [normalize, setNormalize] = useState(false);
  const [targetLUFS, setTargetLUFS] = useState(-14);

  // Playback
  const [isPlaying, setIsPlaying] = useState(false);
  const [playbackPos, setPlaybackPos] = useState(0); // 0..1 within trim region

  // Refs
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const sourceRef = useRef<AudioBufferSourceNode | null>(null);
  const animFrameRef = useRef<number>(0);
  const playbackStartRef = useRef(0);
  const playbackDurationRef = useRef(0);
  const dragRef = useRef<"start" | "end" | "seek" | null>(null);

  const peaks = useMemo(
    () => (audioBuffer ? extractPeaks(audioBuffer, WAVEFORM_POINTS) : null),
    [audioBuffer],
  );

  const totalDuration = audioBuffer ? audioBuffer.duration : 0;
  const trimmedDuration = totalDuration * (trimEnd - trimStart);
  const stretchedDuration = trimmedDuration / speed;

  // Estimate export size (16-bit mono PCM)
  const estimatedBytes = Math.round(stretchedDuration * SAMPLE_RATE * 2 + 44);
  const sizeOk = estimatedBytes <= maxSize;
  const durationOk = stretchedDuration <= maxDuration && stretchedDuration >= minDuration;
  const canExport = audioBuffer && sizeOk && durationOk;

  /* ------ Audio loading ------ */

  const loadAudioData = useCallback(async (data: ArrayBuffer, name: string) => {
    setLoading(true);
    setError(null);
    try {
      const ctx = new AudioContext({ sampleRate: SAMPLE_RATE });
      audioCtxRef.current = ctx;
      const buf = await ctx.decodeAudioData(data);
      setAudioBuffer(buf);
      setFileName(name.replace(/\.[^.]+$/, ""));
      setTrimStart(0);
      setTrimEnd(1);
      setSpeed(1);
      setPlaybackPos(0);
    } catch {
      setError("Failed to decode audio file");
    } finally {
      setLoading(false);
    }
  }, []);

  const handleFileSelect = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file) return;
      const arrayBuf = await file.arrayBuffer();
      await loadAudioData(arrayBuf, file.name);
      e.target.value = "";
    },
    [loadAudioData],
  );

  const handleUrlLoad = useCallback(
    async (url: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await fetch(url);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.arrayBuffer();
        const name = url.split("/").pop() || "audio";
        await loadAudioData(data, name);
      } catch (err) {
        setError((err as Error).message);
        setLoading(false);
      }
    },
    [loadAudioData],
  );

  const handleDrop = useCallback(
    async (e: React.DragEvent) => {
      e.preventDefault();
      const file = e.dataTransfer.files[0];
      if (!file) return;
      const arrayBuf = await file.arrayBuffer();
      await loadAudioData(arrayBuf, file.name);
    },
    [loadAudioData],
  );

  /* ------ Canvas drawing ------ */

  const drawWaveform = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas || !peaks) return;

    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    canvas.width = rect.width * dpr;
    canvas.height = rect.height * dpr;
    ctx.scale(dpr, dpr);

    const w = rect.width;
    const h = rect.height;
    const midY = h / 2;

    // Detect dark mode
    const isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;

    // Background
    ctx.fillStyle = isDark ? "#171717" : "#fafafa";
    ctx.fillRect(0, 0, w, h);

    // Inactive region tint
    const inactiveColor = isDark ? "rgba(0,0,0,0.5)" : "rgba(0,0,0,0.12)";
    ctx.fillStyle = inactiveColor;
    ctx.fillRect(0, 0, w * trimStart, h);
    ctx.fillRect(w * trimEnd, 0, w * (1 - trimEnd), h);

    // Waveform
    const barWidth = w / peaks.length;
    for (let i = 0; i < peaks.length; i++) {
      const x = i * barWidth;
      const fraction = i / peaks.length;
      const inTrim = fraction >= trimStart && fraction <= trimEnd;

      if (inTrim) {
        ctx.fillStyle = isDark ? "#2e78ff" : "#005aff";
      } else {
        ctx.fillStyle = isDark ? "#414141" : "#d0d1d2";
      }

      const amplitude = peaks[i] * midY * 0.9;
      ctx.fillRect(x, midY - amplitude, Math.max(barWidth - 0.5, 0.5), amplitude * 2);
    }

    // Trim markers
    const drawMarker = (pos: number, color: string) => {
      const x = pos * w;
      ctx.strokeStyle = color;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(x, 0);
      ctx.lineTo(x, h);
      ctx.stroke();

      // Handle triangle
      ctx.fillStyle = color;
      ctx.beginPath();
      ctx.moveTo(x - 6, 0);
      ctx.lineTo(x + 6, 0);
      ctx.lineTo(x, 10);
      ctx.closePath();
      ctx.fill();

      ctx.beginPath();
      ctx.moveTo(x - 6, h);
      ctx.lineTo(x + 6, h);
      ctx.lineTo(x, h - 10);
      ctx.closePath();
      ctx.fill();
    };

    drawMarker(trimStart, "#25cb55");
    drawMarker(trimEnd, "#ed4e3b");

    // Playback position
    if (isPlaying || playbackPos > 0) {
      const absPos = trimStart + playbackPos * (trimEnd - trimStart);
      const px = absPos * w;
      ctx.strokeStyle = "#ffbd0a";
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(px, 0);
      ctx.lineTo(px, h);
      ctx.stroke();
    }
  }, [peaks, trimStart, trimEnd, isPlaying, playbackPos]);

  useEffect(() => {
    drawWaveform();
  }, [drawWaveform]);

  // Responsive resize
  useEffect(() => {
    const observer = new ResizeObserver(() => drawWaveform());
    const el = containerRef.current;
    if (el) observer.observe(el);
    return () => observer.disconnect();
  }, [drawWaveform]);

  /* ------ Canvas interaction (drag trim markers) ------ */

  const canvasPositionToFraction = useCallback(
    (clientX: number): number => {
      const canvas = canvasRef.current;
      if (!canvas) return 0;
      const rect = canvas.getBoundingClientRect();
      return Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
    },
    [],
  );

  const handleCanvasMouseDown = useCallback(
    (e: React.MouseEvent) => {
      const frac = canvasPositionToFraction(e.clientX);
      const startDist = Math.abs(frac - trimStart);
      const endDist = Math.abs(frac - trimEnd);
      const threshold = 0.02;

      if (startDist < threshold && startDist <= endDist) {
        dragRef.current = "start";
      } else if (endDist < threshold) {
        dragRef.current = "end";
      } else if (frac > trimStart && frac < trimEnd) {
        // Click inside region = seek
        dragRef.current = "seek";
        const seekFrac = (frac - trimStart) / (trimEnd - trimStart);
        setPlaybackPos(seekFrac);
        if (isPlaying) {
          stopPlayback();
          const time = trimStart * totalDuration + seekFrac * trimmedDuration;
          startPlaybackFrom(time);
        }
      } else {
        // Click outside = move nearest marker
        if (frac < trimStart) {
          setTrimStart(frac);
          dragRef.current = "start";
        } else {
          setTrimEnd(frac);
          dragRef.current = "end";
        }
      }
    },
    [trimStart, trimEnd, isPlaying, totalDuration, trimmedDuration],
  );

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!dragRef.current) return;
      const frac = canvasPositionToFraction(e.clientX);

      if (dragRef.current === "start") {
        setTrimStart(Math.min(frac, trimEnd - 0.005));
      } else if (dragRef.current === "end") {
        setTrimEnd(Math.max(frac, trimStart + 0.005));
      }
    };

    const handleMouseUp = () => {
      dragRef.current = null;
    };

    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("mouseup", handleMouseUp);
    return () => {
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("mouseup", handleMouseUp);
    };
  }, [canvasPositionToFraction, trimStart, trimEnd]);

  /* ------ Playback ------ */

  const stopPlayback = useCallback(() => {
    if (sourceRef.current) {
      try {
        sourceRef.current.stop();
      } catch { /* already stopped */ }
      sourceRef.current = null;
    }
    cancelAnimationFrame(animFrameRef.current);
    setIsPlaying(false);
  }, []);

  const startPlaybackFrom = useCallback(
    (startTimeSec?: number) => {
      if (!audioBuffer || !audioCtxRef.current) return;

      stopPlayback();

      const ctx = audioCtxRef.current;
      const startSample = Math.floor(trimStart * audioBuffer.length);
      const endSample = Math.floor(trimEnd * audioBuffer.length);
      const trimSamples = endSample - startSample;
      if (trimSamples <= 0) return;

      // Extract trimmed region
      const trimmed = new Float32Array(trimSamples);
      const channel = audioBuffer.getChannelData(0);
      for (let i = 0; i < trimSamples; i++) {
        trimmed[i] = channel[startSample + i] || 0;
      }

      // Apply time stretch
      const stretched = wsolaStretch(trimmed, speed);

      // Optionally normalize
      const final = normalize ? normalizeLUFS(stretched, targetLUFS) : stretched;

      // Create buffer
      const playBuf = ctx.createBuffer(1, final.length, SAMPLE_RATE);
      playBuf.getChannelData(0).set(final);

      const source = ctx.createBufferSource();
      source.buffer = playBuf;
      source.connect(ctx.destination);

      const offset = startTimeSec
        ? Math.max(0, startTimeSec - trimStart * totalDuration) / speed
        : 0;

      playbackStartRef.current = ctx.currentTime - offset;
      playbackDurationRef.current = playBuf.duration;

      source.start(0, offset);
      sourceRef.current = source;
      setIsPlaying(true);

      source.onended = () => {
        setIsPlaying(false);
        setPlaybackPos(0);
        cancelAnimationFrame(animFrameRef.current);
      };

      // Animate playback position
      const animate = () => {
        if (!audioCtxRef.current) return;
        const elapsed = audioCtxRef.current.currentTime - playbackStartRef.current;
        const frac = Math.min(1, elapsed / playbackDurationRef.current);
        setPlaybackPos(frac);
        if (frac < 1) {
          animFrameRef.current = requestAnimationFrame(animate);
        }
      };
      animFrameRef.current = requestAnimationFrame(animate);
    },
    [audioBuffer, trimStart, trimEnd, speed, normalize, targetLUFS, totalDuration, stopPlayback],
  );

  const togglePlayback = useCallback(() => {
    if (isPlaying) {
      stopPlayback();
    } else {
      startPlaybackFrom();
    }
  }, [isPlaying, stopPlayback, startPlaybackFrom]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      stopPlayback();
      audioCtxRef.current?.close().catch(() => {});
    };
  }, [stopPlayback]);

  /* ------ Auto-fit ------ */

  const autoFit = useCallback(() => {
    if (!audioBuffer) return;

    let newEnd = trimEnd;
    const durationAtSpeed = (newEnd - trimStart) * totalDuration / speed;
    const bytesAtSpeed = durationAtSpeed * SAMPLE_RATE * 2 + 44;

    // Reduce trim end to fit within constraints
    if (durationAtSpeed > maxDuration || bytesAtSpeed > maxSize) {
      const maxDurBySec = maxDuration * speed;
      const maxDurBySize = ((maxSize - 44) / (SAMPLE_RATE * 2)) * speed;
      const allowedDuration = Math.min(maxDurBySec, maxDurBySize);
      const fraction = allowedDuration / totalDuration;
      newEnd = Math.min(trimStart + fraction, 1);
    }

    if (newEnd !== trimEnd) {
      setTrimEnd(newEnd);
    }
  }, [audioBuffer, trimStart, trimEnd, speed, totalDuration, maxDuration, maxSize]);

  /* ------ Export ------ */

  const handleExport = useCallback(() => {
    if (!audioBuffer) return;

    const startSample = Math.floor(trimStart * audioBuffer.length);
    const endSample = Math.floor(trimEnd * audioBuffer.length);
    const trimmed = new Float32Array(endSample - startSample);
    const channel = audioBuffer.getChannelData(0);
    for (let i = 0; i < trimmed.length; i++) {
      trimmed[i] = channel[startSample + i] || 0;
    }

    const stretched = wsolaStretch(trimmed, speed);
    const final = normalize ? normalizeLUFS(stretched, targetLUFS) : stretched;
    const blob = encodeWAV(final, SAMPLE_RATE);
    onExport(blob, `${fileName}.wav`);
  }, [audioBuffer, trimStart, trimEnd, speed, normalize, targetLUFS, fileName, onExport]);

  /* ------ URL input ------ */

  const [urlInput, setUrlInput] = useState("");

  /* ------ Render ------ */

  return (
    <div className="w-full space-y-4">
      {/* File input area */}
      {!audioBuffer && (
        <div
          onDragOver={(e) => e.preventDefault()}
          onDrop={handleDrop}
          className="flex flex-col items-center gap-4 rounded-sm border-2 border-dashed border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-6 py-10 transition-colors hover:border-[var(--color-text-muted)]"
        >
          <WaveformIcon />
          <p className="text-sm text-[var(--color-text-secondary)]">
            Drop an audio file here or select one below
          </p>
          <div className="flex flex-col sm:flex-row items-center gap-3 w-full max-w-md">
            <label className="flex-1 w-full">
              <input
                type="file"
                accept="audio/*"
                onChange={handleFileSelect}
                className="hidden"
              />
              <span className="flex h-10 w-full cursor-pointer items-center justify-center rounded bg-[var(--color-accent)] px-4 text-sm font-medium text-white hover:bg-[var(--color-accent-hover)] transition-colors">
                Choose File
              </span>
            </label>
            <span className="text-xs text-[var(--color-text-muted)]">or</span>
            <form
              className="flex flex-1 w-full gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (urlInput.trim()) handleUrlLoad(urlInput.trim());
              }}
            >
              <input
                type="url"
                value={urlInput}
                onChange={(e) => setUrlInput(e.target.value)}
                placeholder="https://..."
                className="h-10 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg-card)] px-3 text-sm text-[var(--color-text-primary)] placeholder:text-[var(--color-text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--color-accent)]"
              />
              <button
                type="submit"
                className="h-10 rounded bg-[var(--color-bg-tertiary)] px-3 text-sm font-medium text-[var(--color-text-secondary)] hover:bg-[var(--color-border)] transition-colors"
              >
                Load
              </button>
            </form>
          </div>
        </div>
      )}

      {loading && (
        <div className="flex items-center justify-center gap-2 py-8 text-sm text-[var(--color-text-muted)]">
          <svg className="h-5 w-5 animate-spin" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          Decoding audio...
        </div>
      )}

      {error && (
        <div className="rounded-sm bg-[var(--color-danger-bg)] border border-[var(--color-danger)]/20 px-4 py-3 text-sm text-[var(--color-danger)]">
          {error}
        </div>
      )}

      {audioBuffer && (
        <>
          {/* Waveform canvas */}
          <div ref={containerRef} className="w-full">
            <canvas
              ref={canvasRef}
              onMouseDown={handleCanvasMouseDown}
              className="h-32 w-full cursor-crosshair rounded"
              style={{ touchAction: "none" }}
            />
          </div>

          {/* Info strip */}
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-text-muted)]">
            <span>Source: {totalDuration.toFixed(2)}s</span>
            <span>
              Trim: {(trimStart * totalDuration).toFixed(2)}s &ndash;{" "}
              {(trimEnd * totalDuration).toFixed(2)}s ({trimmedDuration.toFixed(2)}s)
            </span>
            <span>Output: {stretchedDuration.toFixed(2)}s</span>
            <span className={sizeOk ? "" : "text-[var(--color-danger)] font-medium"}>
              ~{formatKB(estimatedBytes)} / {formatKB(maxSize)}
            </span>
          </div>

          {/* Controls grid */}
          <div className="grid gap-4 sm:grid-cols-2">
            {/* Speed */}
            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <label className="text-sm font-medium text-[var(--color-text-primary)]">
                </label>
                <span className="text-sm tabular-nums text-[var(--color-text-muted)]">
                  {speed.toFixed(2)}x
                </span>
              </div>
              <input
                type="range"
                min={speedMin}
                max={speedMax}
                step={speedStep}
                value={speed}
                onChange={(e) => setSpeed(parseFloat(e.target.value))}
                className="w-full accent-[var(--color-accent)]"
              />
              <div className="flex justify-between text-[10px] text-[var(--color-text-muted)]">
                <span>{speedMin}x</span>
                <span>1x</span>
                <span>{speedMax}x</span>
              </div>
            </div>

            {/* Normalization */}
            <div className="space-y-1.5">
              <div className="flex items-center gap-3">
                <label className="text-sm font-medium text-[var(--color-text-primary)]">
                  Normalize
                </label>
                <button
                  onClick={() => setNormalize(!normalize)}
                  className={`relative h-5 w-9 rounded-full transition-colors ${
                    normalize ? "bg-[var(--color-accent)]" : "bg-[var(--color-border)]"
                  }`}
                >
                  <span
                    className={`absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform ${
                      normalize ? "translate-x-4" : "translate-x-0.5"
                    }`}
                  />
                </button>
              </div>
              {normalize && (
                <div className="flex items-center gap-2">
                  <label className="text-xs text-[var(--color-text-muted)]">Target LUFS:</label>
                  <input
                    type="number"
                    value={targetLUFS}
                    onChange={(e) => setTargetLUFS(parseFloat(e.target.value) || -14)}
                    step={1}
                    min={-30}
                    max={0}
                    className="h-7 w-20 rounded border border-[var(--color-border)] bg-[var(--color-bg-card)] px-2 text-xs text-[var(--color-text-primary)] focus:outline-none focus:ring-1 focus:ring-[var(--color-accent)]"
                  />
                </div>
              )}
            </div>
          </div>

          {/* Action buttons */}
          <div className="flex flex-wrap items-center gap-2">
            <button
              onClick={togglePlayback}
              className="flex items-center gap-1.5 rounded bg-[var(--color-bg-tertiary)] px-4 py-2 text-sm font-medium text-[var(--color-text-primary)] hover:bg-[var(--color-border)] transition-colors"
            >
              {isPlaying ? (
                <>
                  <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" />
                  </svg>
                  Stop
                </>
              ) : (
                <>
                  <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M8 5v14l11-7z" />
                  </svg>
                  Preview
                </>
              )}
            </button>

            <button
              onClick={autoFit}
              className="rounded border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)] transition-colors"
            >
              Auto-fit
            </button>

            <button
              onClick={handleExport}
              disabled={!canExport}
              className={`ml-auto flex items-center gap-1.5 rounded px-5 py-2 text-sm font-medium transition-colors ${
                canExport
                  ? "bg-[var(--color-success)] text-white hover:opacity-90"
                  : "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)] cursor-not-allowed"
              }`}
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
              Export WAV
            </button>

            <button
              onClick={() => {
                setAudioBuffer(null);
                setFileName("audio");
                setTrimStart(0);
                setTrimEnd(1);
                setSpeed(1);
                setPlaybackPos(0);
                setError(null);
                stopPlayback();
              }}
              className="rounded border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)] transition-colors"
            >
              Clear
            </button>
          </div>

          {/* Constraint warnings */}
          {(!sizeOk || !durationOk) && (
            <div className="rounded-sm bg-[var(--color-warning-bg)] border border-[var(--color-warning)]/20 px-4 py-3 text-sm text-[var(--color-warning)]">
              {!sizeOk && (
                <p>
                  File too large ({formatKB(estimatedBytes)}). Maximum: {formatKB(maxSize)}.
                  Try trimming shorter or increasing speed.
                </p>
              )}
              {!durationOk && stretchedDuration > maxDuration && (
                <p>
                  Duration too long ({stretchedDuration.toFixed(1)}s). Maximum: {maxDuration}s.
                </p>
              )}
              {!durationOk && stretchedDuration < minDuration && (
                <p>
                  Duration too short ({stretchedDuration.toFixed(2)}s). Minimum: {minDuration}s.
                </p>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function formatKB(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

function WaveformIcon() {
  return (
    <svg
      className="h-10 w-10 text-[var(--color-text-muted)]"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M2 12h2l2-5 3 10 3-8 2 6 2-3h2l2-4 2 4h2" />
    </svg>
  );
}
