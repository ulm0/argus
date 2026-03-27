"use client";

import { useEffect, useState, useCallback } from "react";
import * as api from "@/lib/api";
import type { VideoEvent } from "@/lib/types";
import DashcamPlayer from "@/components/DashcamPlayer";

interface Props {
  folder: string;
  event: string;
}

export default function VideoEventPage({ folder, event }: Props) {
  const [details, setDetails] = useState<VideoEvent | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deleted, setDeleted] = useState(false);

  useEffect(() => {
    setLoading(true);
    setError(null);
    api
      .getEvent(folder, event)
      .then(setDetails)
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load event"))
      .finally(() => setLoading(false));
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
        <a
          href={api.downloadEventURL(folder, event)}
          className="ml-auto rounded-sm border border-[var(--color-border)] px-3 py-1 text-xs font-medium text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)] transition-colors"
        >
          Download ZIP
        </a>
      </div>

      {/* Player */}
      <div className="overflow-hidden rounded-lg">
        <DashcamPlayer
          event={details}
          streamUrlFn={(file) => api.streamURL(`${folder}/${event}/${file}`)}
          seiUrlFn={(file) => api.seiURL(`${folder}/${event}/${file}`)}
          onDelete={handleDelete}
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
