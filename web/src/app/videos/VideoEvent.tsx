"use client";

import { useEffect, useState, useCallback } from "react";
import * as api from "@/lib/api";
import type { AppStatus, VideoEvent, VideoEventsResponse } from "@/lib/types";
import DashcamPlayer from "@/components/DashcamPlayer";

interface Props {
  folder: string;
  event: string;
}

export default function VideoEventPage({ folder, event }: Props) {
  const [details, setDetails] = useState<VideoEvent | null>(null);
  const [appStatus, setAppStatus] = useState<AppStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deleted, setDeleted] = useState(false);
  const [prevEvent, setPrevEvent] = useState<VideoEvent | null>(null);
  const [nextEvent, setNextEvent] = useState<VideoEvent | null>(null);

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

  useEffect(() => {
    let cancelled = false;

    const loadAdjacentEvents = async () => {
      setPrevEvent(null);
      setNextEvent(null);

      try {
        let page = 0;
        let prevCandidate: VideoEvent | null = null;
        let foundCurrent = false;

        while (!cancelled) {
          const res = (await api.getVideos(folder, page)) as VideoEventsResponse;
          const pageEvents = res.events ?? [];

          for (const ev of pageEvents) {
            if (foundCurrent) {
              if (!cancelled) setNextEvent(ev);
              return;
            }
            if (ev.name === event) {
              if (prevCandidate && !cancelled) setPrevEvent(prevCandidate);
              foundCurrent = true;
              continue;
            }
            prevCandidate = ev;
          }

          if (!res.has_next) return;
          page += 1;
        }
      } catch {
        // Leave both events as null when lookup fails.
      }
    };

    void loadAdjacentEvents();
    return () => {
      cancelled = true;
    };
  }, [folder, event]);

  const handleDelete = useCallback(async () => {
    if (!confirm(`Delete event "${event}"?`)) return;
    try {
      await api.deleteEvent(folder, event);
      setDeleted(true);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  }, [folder, event]);

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
          href={`/videos/${encodeURIComponent(folder)}`}
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
          <span className="rounded bg-[var(--color-accent-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-accent-text)]">
            {details.reason}
          </span>
        )}
        {details.city && (
          <span className="text-sm text-[var(--color-text-muted)]">{details.city}</span>
        )}
        {details.datetime && (
          <span className="text-sm text-[var(--color-text-muted)]">
            {formatDate(details.datetime)}
          </span>
        )}
        <span className="text-sm text-[var(--color-text-muted)]">
          {(details.size_mb).toFixed(1)} MB
        </span>
        {prevEvent && (
          <a
            href={`/videos/${encodeURIComponent(folder)}/${encodeURIComponent(prevEvent.name)}`}
            title="Go to previous event"
            className="ml-auto flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-bg-secondary)] p-1 pl-3 text-xs transition-colors hover:bg-[var(--color-bg-tertiary)]"
          >
            <span className="flex flex-col leading-tight text-right">
              <span className="text-sm font-semibold text-[var(--color-text-primary)]">
                {prevEvent.city || prevEvent.name}
              </span>
              {prevEvent.datetime && (
                <span className="text-[11px] text-[var(--color-text-muted)]">
                  {formatShortDate(prevEvent.datetime)}
                </span>
              )}
            </span>
            {prevEvent.has_thumbnail && (
              <img
                src={api.thumbnailURL(folder, prevEvent.name)}
                alt=""
                className="h-9 w-16 rounded object-cover"
                loading="lazy"
                onError={(e) => {
                  (e.currentTarget as HTMLImageElement).style.display = "none";
                }}
              />
            )}
            <span className="flex h-7 w-7 items-center justify-center rounded text-[var(--color-text-secondary)]">
              <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                <path d="M6 6h2v12H6zm3.5 6L18 6v12z" />
              </svg>
            </span>
          </a>
        )}
        {nextEvent && (
          <a
            href={`/videos/${encodeURIComponent(folder)}/${encodeURIComponent(nextEvent.name)}`}
            title="Go to next event"
            className={`${prevEvent ? "" : "ml-auto "}flex items-center gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-bg-secondary)] p-1 pr-3 text-xs transition-colors hover:bg-[var(--color-bg-tertiary)]`}
          >
            <span className="flex h-7 w-7 items-center justify-center rounded text-[var(--color-text-secondary)]">
              <svg className="h-4 w-4" fill="currentColor" viewBox="0 0 24 24">
                <path d="M6 18l8.5-6L6 6v12zM16 6h2v12h-2z" />
              </svg>
            </span>
            {nextEvent.has_thumbnail && (
              <img
                src={api.thumbnailURL(folder, nextEvent.name)}
                alt=""
                className="h-9 w-16 rounded object-cover"
                loading="lazy"
                onError={(e) => {
                  (e.currentTarget as HTMLImageElement).style.display = "none";
                }}
              />
            )}
            <span className="flex flex-col leading-tight">
              <span className="text-sm font-semibold text-[var(--color-text-primary)]">
                {nextEvent.city || nextEvent.name}
              </span>
              {nextEvent.datetime && (
                <span className="text-[11px] text-[var(--color-text-muted)]">
                  {formatShortDate(nextEvent.datetime)}
                </span>
              )}
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
          downloadZipUrl={api.downloadEventURL(folder, event)}
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

function formatShortDate(ts: string): string {
  if (!ts) return "";
  try {
    return new Date(ts).toLocaleDateString(undefined, {
      weekday: "short",
      month: "short",
      day: "numeric",
      year: "numeric",
    });
  } catch {
    return ts;
  }
}
