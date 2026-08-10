"use client";

import { useEffect, useState, useCallback } from "react";
import * as api from "@/lib/api";
import type { AppStatus, CameraName, VideoEvent } from "@/lib/types";
import DashcamPlayer from "@/components/DashcamPlayer";
import { useToast } from "@/components/Toast";
import { cameraFromIndex, formatEventReason } from "@/lib/eventReason";
import { formatMegabytes } from "@/lib/format";

interface Props {
  folder: string;
  event: string;
}

// Archive folders are two path segments ("archive/SavedClips"), so each has to
// be encoded on its own or the slash turns into %2F and the route breaks.
function folderHref(folder: string): string {
  return folder.split("/").map(encodeURIComponent).join("/");
}

export default function VideoEventPage({ folder, event }: Props) {
  const { showToast } = useToast();
  const [details, setDetails] = useState<VideoEvent | null>(null);
  const [appStatus, setAppStatus] = useState<AppStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deleted, setDeleted] = useState(false);

  // ?clip=&t= carries a position to another device or across a reload.
  const [initialPos] = useState(() => {
    if (typeof window === "undefined") return {};
    const q = new URLSearchParams(window.location.search);
    const clip = q.get("clip");
    const t = q.get("t");
    return {
      clip: clip !== null && Number.isFinite(Number(clip)) ? Number(clip) : undefined,
      t: t !== null && Number.isFinite(Number(t)) ? Number(t) : undefined,
    };
  });

  const handlePosition = useCallback((clip: number, t: number) => {
    const q = new URLSearchParams(window.location.search);
    q.set("clip", String(clip));
    q.set("t", t.toFixed(1));
    window.history.replaceState(null, "", `${window.location.pathname}?${q}`);
  }, []);

  useEffect(() => {
    setLoading(true);
    setError(null);
    Promise.all([api.getEvent(folder, event), api.getStatus()])
      .then(([eventData, statusData]) => {
        setDetails(eventData);
        setAppStatus(statusData);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load event"))
      .finally(() => setLoading(false));
  }, [folder, event]);

  const handleDelete = useCallback(async () => {
    if (!confirm(`Delete event "${event}"?`)) return;
    try {
      await api.deleteEvent(folder, event);
      setDeleted(true);
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Delete failed", false);
    }
  }, [folder, event, showToast]);

  if (deleted) {
    return (
      <div className="flex min-h-full flex-col items-center justify-center gap-4 p-8">
        <p className="text-[var(--color-text-secondary)]">Event deleted.</p>
        <a
          href="/videos"
          className="rounded-sm bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
        >
          Back to Videos
        </a>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  if (error || !details) {
    return (
      <div className="flex min-h-full flex-col items-center justify-center gap-4 p-8">
        <p className="text-[var(--color-danger)]">{error ?? "Event not found"}</p>
        <a
          href="/videos"
          className="rounded-sm bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
        >
          Back to Videos
        </a>
      </div>
    );
  }

  return (
    <div className="w-full space-y-4 p-4 lg:p-6">
      {/* Breadcrumb */}
      <nav className="flex items-center gap-2 text-sm text-[var(--color-text-muted)]">
        <a href="/videos" className="hover:text-[var(--color-text-primary)] transition-colors">
          Videos
        </a>
        <span>/</span>
        <a
          href={`/videos/${folderHref(folder)}`}
          className="hover:text-[var(--color-text-primary)] transition-colors"
        >
          {folder}
        </a>
        <span>/</span>
        <span className="text-[var(--color-text-primary)] font-medium truncate max-w-[16rem]">
          {event}
        </span>
      </nav>

      {/* Event metadata */}
      <div className="flex flex-wrap items-center gap-3">
        {details.reason && (
          <span
            className="rounded bg-[var(--color-accent-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-accent-text)]"
            title={details.reason}
          >
            {formatEventReason(details.reason)}
          </span>
        )}
        {details.city && (
          <span className="text-sm text-[var(--color-text-muted)]">{details.city}</span>
        )}
        {details.est_lat !== undefined && details.est_lon !== undefined && (
          // Plain text, not a map link: the Pi's AP has no internet. For an
          // encrypted or corrupt clip this is the only location evidence.
          <span className="text-sm tabular-nums text-[var(--color-text-muted)]">
            {details.est_lat.toFixed(5)}, {details.est_lon.toFixed(5)}
          </span>
        )}
        {details.datetime && (
          <span className="text-sm text-[var(--color-text-muted)]">
            {formatDate(details.datetime)}
          </span>
        )}
        <span className="text-sm text-[var(--color-text-muted)]" title={`${details.size_mb.toFixed(1)} MB`}>
          {formatMegabytes(details.size_mb)}
        </span>
        {/* Neighbours come from the event payload — the backend knows the
            folder order, so the browser no longer pages through it. */}
        {details.prev && (
          <a
            href={`/videos/${folderHref(folder)}/${encodeURIComponent(details.prev)}`}
            title="Go to previous event"
            className="ml-auto flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-bg-secondary)] p-1 pl-3 text-xs transition-colors hover:bg-[var(--color-bg-tertiary)]"
          >
            <span className="text-sm font-semibold text-[var(--color-text-primary)]">
              {details.prev}
            </span>
            <img
              src={api.thumbnailURL(folder, details.prev)}
              alt=""
              className="h-9 w-16 rounded object-cover"
              loading="lazy"
              onError={(e) => {
                (e.currentTarget as HTMLImageElement).style.display = "none";
              }}
            />
            <span className="flex h-7 w-7 items-center justify-center rounded text-[var(--color-text-secondary)]">
              <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                <path d="M6 6h2v12H6zm3.5 6L18 6v12z" />
              </svg>
            </span>
          </a>
        )}
        {details.next && (
          <a
            href={`/videos/${folderHref(folder)}/${encodeURIComponent(details.next)}`}
            title="Go to next event"
            className={`${details.prev ? "" : "ml-auto "}flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-bg-secondary)] p-1 pr-3 text-xs transition-colors hover:bg-[var(--color-bg-tertiary)]`}
          >
            <span className="flex h-7 w-7 items-center justify-center rounded text-[var(--color-text-secondary)]">
              <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                <path d="M6 18l8.5-6L6 6v12zM16 6h2v12h-2z" />
              </svg>
            </span>
            <img
              src={api.thumbnailURL(folder, details.next)}
              alt=""
              className="h-9 w-16 rounded object-cover"
              loading="lazy"
              onError={(e) => {
                (e.currentTarget as HTMLImageElement).style.display = "none";
              }}
            />
            <span className="text-sm font-semibold text-[var(--color-text-primary)]">
              {details.next}
            </span>
          </a>
        )}
      </div>

      {/* Player */}
      <div className="overflow-hidden rounded-lg">
        <DashcamPlayer
          event={details}
          streamUrlFn={(file) => api.streamURL(`${folder}/${event}/${file}`)}
          telemetryUrlFn={(file) => api.telemetryURL(`${folder}/${event}/${file}`)}
          downloadUrlFn={(file) => api.downloadURL(`${folder}/${event}/${file}`)}
          exportUrlFn={(file, start, end) =>
            api.exportURL(`${folder}/${event}/${file}`, start, end)
          }
          thumbnailUrlFn={(cam) => api.thumbnailURL(folder, event, cam)}
          downloadZipUrl={api.downloadEventURL(folder, event)}
          initialCamera={cameraFromIndex(details.camera) as CameraName | undefined}
          initialClipIndex={initialPos.clip}
          initialTime={initialPos.t}
          onPositionChange={handlePosition}
          onDelete={appStatus?.mode === "edit" ? handleDelete : undefined}
        />
      </div>
    </div>
  );
}

function formatDate(ts: string): string {
  if (!ts) return "";
  try {
    return new Date(ts).toLocaleDateString(undefined, {
      month: "short",
      day: "numeric",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return ts;
  }
}
