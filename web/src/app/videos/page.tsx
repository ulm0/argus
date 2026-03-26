"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import * as api from "@/lib/api";
import type {
  VideoFolder,
  VideoEvent,
  SessionGroup,
  VideoEventsResponse,
  VideoSessionsResponse,
  VideoListResponse,
} from "@/lib/types";

const FOLDER_TABS = ["SavedClips", "SentryClips", "RecentClips"] as const;
type FolderTab = (typeof FOLDER_TABS)[number];

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

function formatDate(ts: string): string {
  if (!ts) return "—";
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

export default function VideosPage() {
  const [folders, setFolders] = useState<VideoFolder[]>([]);
  const [activeTab, setActiveTab] = useState<FolderTab>("SavedClips");
  const [events, setEvents] = useState<VideoEvent[]>([]);
  const [sessions, setSessions] = useState<SessionGroup[]>([]);
  const [page, setPage] = useState(0);
  const [hasNext, setHasNext] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [toast, setToast] = useState<{ msg: string; ok: boolean } | null>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  const showToast = useCallback((msg: string, ok = true) => {
    setToast({ msg, ok });
    setTimeout(() => setToast(null), 3000);
  }, []);

  useEffect(() => {
    api
      .getVideos()
      .then((res) => {
        const data = res as VideoListResponse;
        setFolders(data.folders || []);
      })
      .catch(() => showToast("Failed to load folders", false))
      .finally(() => setLoading(false));
  }, [showToast]);

  const loadEvents = useCallback(
    async (folder: FolderTab, pageNum: number, append = false) => {
      if (!append) setLoadingMore(false);
      else setLoadingMore(true);

      try {
        if (folder === "RecentClips") {
          const res = (await api.getVideos(
            folder,
            pageNum,
            20,
            "sessions",
          )) as VideoSessionsResponse;
          setSessions((prev) =>
            append ? [...prev, ...(res.sessions || [])] : res.sessions || [],
          );
          setEvents([]);
          setHasNext(res.has_next);
        } else {
          const res = (await api.getVideos(
            folder,
            pageNum,
          )) as VideoEventsResponse;
          setEvents((prev) =>
            append ? [...prev, ...(res.events || [])] : res.events || [],
          );
          setSessions([]);
          setHasNext(res.has_next);
        }
        setPage(pageNum);
      } catch {
        showToast("Failed to load events", false);
      } finally {
        setLoadingMore(false);
      }
    },
    [showToast],
  );

  useEffect(() => {
    setEvents([]);
    setSessions([]);
    setPage(0);
    setHasNext(false);
    loadEvents(activeTab, 0);
  }, [activeTab, loadEvents]);

  useEffect(() => {
    if (!hasNext || loadingMore) return;
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasNext && !loadingMore) {
          loadEvents(activeTab, page + 1, true);
        }
      },
      { threshold: 0.1 },
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [hasNext, loadingMore, page, activeTab, loadEvents]);

  const handleDelete = async (event: string) => {
    if (!confirm(`Delete event "${event}"?`)) return;
    setDeleting(event);
    try {
      await api.deleteEvent(activeTab, event);
      setEvents((prev) => prev.filter((e) => e.name !== event));
      showToast("Event deleted");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Delete failed", false);
    } finally {
      setDeleting(null);
    }
  };

  const folderInfo = (tab: FolderTab) =>
    folders.find((f) => f.name === tab);

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      {toast && (
        <div
          className={`fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-sm px-4 py-3 text-sm font-medium shadow-lg ${
            toast.ok ? "bg-[var(--color-success)] text-white" : "bg-[var(--color-danger)] text-white"
          }`}
        >
          {toast.msg}
        </div>
      )}

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">
          Videos
        </h1>
        <button
          onClick={() => setEditMode(!editMode)}
          className={`rounded-sm px-3 py-1.5 text-sm font-medium transition-colors ${
            editMode
              ? "bg-[var(--color-danger-bg)] text-[var(--color-danger)]"
              : "border border-[var(--color-border)] text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]"
          }`}
        >
          {editMode ? "Done" : "Edit"}
        </button>
      </div>

      {/* Folder Tabs — Tesla-style segmented control */}
      <div className="flex gap-1 rounded bg-[var(--color-bg-tertiary)] p-1">
        {FOLDER_TABS.map((tab) => {
          const info = folderInfo(tab);
          return (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`flex-1 rounded px-4 py-2 text-sm font-medium transition-all ${
                activeTab === tab
                  ? "bg-[var(--color-accent)] text-white shadow-sm"
                  : "text-[var(--color-text-secondary)] hover:text-[var(--color-text-primary)]"
              }`}
            >
              <span className="block">{tab.replace("Clips", "")}</span>
              {info && (
                <span className={`block text-xs ${
                  activeTab === tab ? "text-white/70" : "text-[var(--color-text-muted)]"
                }`}>
                  {info.count} events
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* Events Grid */}
      {activeTab === "RecentClips" ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {sessions.map((s) => (
            <div
              key={s.session}
              className="group relative overflow-hidden rounded bg-[var(--color-bg-card)] shadow-sm transition-all hover:shadow-md"
            >
              <div className="aspect-video bg-[var(--color-bg-tertiary)]">
                <img
                  src={`/api/videos/session-thumbnail/${activeTab}/${s.session}`}
                  alt={s.session}
                  className="h-full w-full object-cover"
                  loading="lazy"
                  onError={(e) => {
                    (e.target as HTMLImageElement).style.display = "none";
                  }}
                />
              </div>
              <div className="p-4">
                <p className="text-sm font-semibold text-[var(--color-text-primary)]">
                  {s.session}
                </p>
                <div className="mt-1 flex items-center gap-3 text-xs text-[var(--color-text-muted)]">
                  <span>{formatDate(s.timestamp)}</span>
                  <span>{s.cameras?.length || 0} cameras</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {events.map((ev) => (
            <div
              key={ev.name}
              className="group relative overflow-hidden rounded bg-[var(--color-bg-card)] shadow-sm transition-all hover:shadow-md"
            >
              <a href={`/videos/${activeTab}/${ev.name}`} className="block">
                <div className="aspect-video bg-[var(--color-bg-tertiary)]">
                  <img
                    src={api.thumbnailURL(activeTab, ev.name)}
                    alt={ev.name}
                    className="h-full w-full object-cover"
                    loading="lazy"
                    onError={(e) => {
                      (e.target as HTMLImageElement).style.display = "none";
                    }}
                  />
                </div>
                <div className="p-4">
                  <div className="flex items-start justify-between">
                    <p className="text-sm font-semibold text-[var(--color-text-primary)]">
                      {ev.name}
                    </p>
                    {ev.reason && (
                      <span className="ml-2 shrink-0 rounded bg-[var(--color-accent-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-accent-text)]">
                        {ev.reason}
                      </span>
                    )}
                  </div>
                  <div className="mt-1 flex items-center gap-3 text-xs text-[var(--color-text-muted)]">
                    <span>{formatDate(ev.datetime)}</span>
                    <span>{formatBytes(ev.size_mb * 1024 * 1024)}</span>
                  </div>
                  {ev.city && (
                    <p className="mt-1 text-xs text-[var(--color-text-muted)]">{ev.city}</p>
                  )}
                </div>
              </a>

              {editMode && (
                <div className="absolute right-2 top-2 flex gap-1">
                  <a
                    href={api.downloadEventURL(activeTab, ev.name)}
                    className="rounded bg-black/60 p-1.5 text-white backdrop-blur-sm transition-colors hover:bg-black/80"
                    title="Download ZIP"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M12 10v6m0 0l-3-3m3 3l3-3M3 17V7a2 2 0 012-2h6l2 2h6a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z" />
                    </svg>
                  </a>
                  <button
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      handleDelete(ev.name);
                    }}
                    disabled={deleting === ev.name}
                    className="rounded bg-[var(--color-danger)] p-1.5 text-white backdrop-blur-sm transition-colors hover:opacity-90 disabled:opacity-50"
                    title="Delete"
                  >
                    <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {events.length === 0 && sessions.length === 0 && !loading && (
        <div className="py-12 text-center text-sm text-[var(--color-text-muted)]">
          No events found in {activeTab}
        </div>
      )}

      <div ref={sentinelRef} className="h-4" />

      {loadingMore && (
        <div className="flex justify-center py-4">
          <div className="h-6 w-6 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
        </div>
      )}
    </div>
  );
}
