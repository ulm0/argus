"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { usePathname } from "next/navigation";
import * as api from "@/lib/api";
import type {
  AppStatus,
  VideoFolder,
  VideoEvent,
  SessionGroup,
  VideoEventsResponse,
  VideoSessionsResponse,
  VideoListResponse,
} from "@/lib/types";
import { useToast } from "@/components/Toast";
import { eventReasonKind, formatEventReason } from "@/lib/eventReason";
import { formatMegabytes } from "@/lib/format";

const DEFAULT_TAB = "SavedClips";

// Reason chips filter the already-loaded page client-side — no extra work for
// the Pi.
const REASON_FILTERS = ["All", "Sentry", "Honk", "Saved"] as const;
type ReasonFilter = (typeof REASON_FILTERS)[number];

function matchesReason(raw: string | undefined, filter: ReasonFilter): boolean {
  if (filter === "All") return true;
  // A honk is a user_* value too, so it is tested on its label and then kept
  // out of Saved — everything else user_* is a manual save.
  const isHonk = formatEventReason(raw) === "Honk";
  if (filter === "Honk") return isHonk;
  // Bucket by prefix, not by the label table: a `sentry_*` value Tesla only
  // just invented still belongs under Sentry instead of silently vanishing
  // from the list the moment someone filters.
  const kind = eventReasonKind(raw);
  if (filter === "Sentry") return kind === "sentry";
  return kind === "user" && !isHonk;
}

// The session id and the folder both come off the drive, so a name holding "#"
// or "?" has to be percent-encoded per segment or the request truncates.
// api.ts's own encodePath isn't exported, so this mirrors it at the call site.
function sessionThumbnailURL(folder: string, session: string): string {
  const path = folder.split("/").map(encodeURIComponent).join("/");
  return `/api/videos/session-thumbnail/${path}/${encodeURIComponent(session)}`;
}

// /videos and /videos/{folder} both render this component (see app/videos/page.tsx).
// Pull the folder out of the path so the breadcrumb link in VideoEvent
// (e.g. /videos/SentryClips) actually lands on the matching tab instead of
// silently defaulting to SavedClips. Archive folders are two segments.
function tabFromPath(pathname: string | null): string {
  if (!pathname) return DEFAULT_TAB;
  const parts = pathname.replace(/^\/videos\/?/, "").split("/").filter(Boolean);
  if (!parts.length) return DEFAULT_TAB;
  return parts[0] === "archive" && parts[1] ? `${parts[0]}/${parts[1]}` : parts[0];
}

// Tabs come from whatever directories exist on the drive, which on a FAT/exFAT
// stick includes filesystem bookkeeping nobody recorded. Dot-directories cover
// .Trash-1000/.Spotlight-V100/.fseventsd; the rest are Windows' own.
const JUNK_DIRS = new Set([
  "System Volume Information",
  "$RECYCLE.BIN",
  "RECYCLER",
  "found.000",
  "LOST.DIR",
]);

function isRealFolder(f: VideoFolder): boolean {
  const base = f.path.split("/").pop() ?? f.path;
  return !base.startsWith(".") && !JUNK_DIRS.has(base);
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

export default function VideosListPage() {
  const pathname = usePathname();
  const { showToast } = useToast();
  const [folders, setFolders] = useState<VideoFolder[]>([]);
  // Empty means the TeslaCam drive isn't mounted — a different story from
  // "this folder has no events".
  const [teslacamPath, setTeslacamPath] = useState<string | null>(null);
  const [status, setStatus] = useState<AppStatus | null>(null);
  const [tab, setTab] = useState<string>(() => tabFromPath(pathname));
  const [before, setBefore] = useState("");
  const [reasonFilter, setReasonFilter] = useState<ReasonFilter>("All");
  const [selected, setSelected] = useState<Set<string>>(new Set());

  // Keep the active tab in sync with the URL when the user clicks the
  // breadcrumb back to /videos/{folder} from an event detail page.
  useEffect(() => {
    const next = tabFromPath(pathname);
    setTab((prev) => (prev === next ? prev : next));
  }, [pathname]);

  const selectTab = useCallback((tab: string) => {
    setTab(tab);
    setSelected(new Set());
    // Reflect the tab in the URL so the breadcrumb stays consistent and
    // back/forward navigation feels right. We use replaceState rather than
    // pushState so each tab click doesn't accumulate history entries.
    if (typeof window !== "undefined") {
      window.history.replaceState(null, "", `/videos/${tab}`);
    }
  }, []);
  const [events, setEvents] = useState<VideoEvent[]>([]);
  const [sessions, setSessions] = useState<SessionGroup[]>([]);
  const [page, setPage] = useState(0);
  const [hasNext, setHasNext] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadingEvents, setLoadingEvents] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const [editMode, setEditMode] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [bulkBusy, setBulkBusy] = useState(false);
  const [thumbFailed, setThumbFailed] = useState<Set<string>>(new Set());
  const sentinelRef = useRef<HTMLDivElement>(null);
  const reqRef = useRef(0);

  // The URL can name a folder the device doesn't have (a stale link, or
  // /videos/local); fall back to the first real one rather than querying it.
  const activeTab =
    !folders.length || folders.some((f) => f.path === tab) ? tab : folders[0].path;
  const isRecent = activeTab.endsWith("RecentClips");
  const canEdit = status?.mode === "edit";

  const loadFolders = useCallback(() => {
    api
      .getVideos()
      .then((res) => {
        const data = res as VideoListResponse;
        setFolders((data.folders || []).filter(isRealFolder));
        setTeslacamPath(data.teslacam_path ?? "");
        setLoadError(false);
      })
      .catch(() => setLoadError(true))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    loadFolders();
    // Deleting is rejected outside edit mode server-side, so the controls that
    // would 403 stay hidden until the device is actually in edit mode.
    api.getStatus().then(setStatus).catch(() => {});
  }, [loadFolders]);

  const loadEvents = useCallback(
    async (folder: string, pageNum: number, append = false) => {
      const reqId = ++reqRef.current;
      if (!append) {
        setLoadingMore(false);
        setLoadingEvents(true);
      } else setLoadingMore(true);

      try {
        if (folder.endsWith("RecentClips")) {
          const res = (await api.getVideos(
            folder,
            pageNum,
            20,
            "sessions",
          )) as VideoSessionsResponse;
          if (reqId !== reqRef.current) return;
          setSessions((prev) =>
            append ? [...prev, ...(res.sessions || [])] : res.sessions || [],
          );
          setEvents([]);
          setHasNext(res.has_next);
        } else {
          const res = (await api.getVideos(
            folder,
            pageNum,
            20,
            undefined,
            before || undefined,
          )) as VideoEventsResponse;
          if (reqId !== reqRef.current) return;
          setEvents((prev) =>
            append ? [...prev, ...(res.events || [])] : res.events || [],
          );
          setSessions([]);
          setHasNext(res.has_next);
        }
        setPage(pageNum);
        setLoadError(false);
      } catch {
        if (reqId !== reqRef.current) return;
        // Without this the empty state would claim the clips don't exist —
        // a lie worth avoiding right after an incident.
        setLoadError(true);
      } finally {
        if (reqId === reqRef.current) {
          setLoadingMore(false);
          setLoadingEvents(false);
        }
      }
    },
    [before],
  );

  useEffect(() => {
    if (!folders.length) return;
    setEvents([]);
    setSessions([]);
    setPage(0);
    setHasNext(false);
    loadEvents(activeTab, 0);
    // folders.length is a dependency: activeTab often doesn't change when the
    // folder list arrives, but that's the point at which loading may proceed.
  }, [activeTab, folders.length, loadEvents]);

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

  const deleteSelected = async () => {
    const names = [...selected];
    if (!names.length || !confirm(`Delete ${names.length} events?`)) return;
    setBulkBusy(true);
    let failed = 0;
    // Sequential: the Pi has one core and no bulk endpoint.
    for (const name of names) {
      try {
        await api.deleteEvent(activeTab, name);
        setEvents((prev) => prev.filter((e) => e.name !== name));
      } catch {
        failed += 1;
      }
    }
    setSelected(new Set());
    setBulkBusy(false);
    showToast(
      failed
        ? `Deleted ${names.length - failed}, ${failed} failed`
        : `Deleted ${names.length} events`,
      failed === 0,
    );
  };

  const toggleKeep = async (name: string) => {
    try {
      const res = await api.keepEvent(activeTab, name);
      setEvents((prev) =>
        prev.map((e) => (e.name === name ? { ...e, kept: res.kept } : e)),
      );
      showToast(res.kept ? "Pinned — cleanup will skip it" : "Unpinned");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Failed to pin", false);
    }
  };

  const toggleSelected = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const markThumbFailed = (key: string) =>
    setThumbFailed((prev) => (prev.has(key) ? prev : new Set(prev).add(key)));

  const visibleEvents = events.filter((ev) => matchesReason(ev.reason, reasonFilter));

  if (loading) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      </div>
    );
  }

  return (
    <div className="w-full space-y-6 p-6 lg:p-8">
      <div className="flex items-center justify-between gap-2">
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">
          Videos
        </h1>
        <a
          href="/videos/local"
          className="rounded-sm border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)] hover:bg-[var(--color-bg-secondary)] transition-colors"
          title="Analyze local MP4 files from your computer"
        >
          Local Analysis
        </a>
        {canEdit ? (
          <button
            onClick={() => {
              setEditMode(!editMode);
              setSelected(new Set());
            }}
            className={`rounded-sm px-3 py-1.5 text-sm font-medium transition-colors ${
              editMode
                ? "bg-[var(--color-danger-bg)] text-[var(--color-danger)]"
                : "border border-[var(--color-border)] text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]"
            }`}
          >
            {editMode ? "Done" : "Edit"}
          </button>
        ) : (
          <span className="text-xs text-[var(--color-text-muted)]">
            Switch the device to Edit mode to delete clips
          </span>
        )}
      </div>

      {/* Folder Tabs — rendered from the API so archive folders show up too */}
      <div className="flex gap-1 overflow-x-auto rounded bg-[var(--color-bg-tertiary)] p-1">
        {folders.map((f) => (
          <button
            key={f.path}
            onClick={() => selectTab(f.path)}
            className={`flex-1 shrink-0 whitespace-nowrap rounded px-4 py-2 text-sm font-medium transition-all ${
              activeTab === f.path
                ? "bg-[var(--color-accent)] text-white shadow-sm"
                : "text-[var(--color-text-secondary)] hover:text-[var(--color-text-primary)]"
            }`}
          >
            <span className="block">{f.name.replace("Clips", "")}</span>
            <span
              className={`block text-xs ${
                activeTab === f.path ? "text-white/70" : "text-[var(--color-text-muted)]"
              }`}
            >
              {f.count} events
            </span>
          </button>
        ))}
      </div>

      {/* Find controls — "my car was hit around 3pm Tuesday" */}
      {!isRecent && (
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-2 text-xs text-[var(--color-text-muted)]">
            Jump to
            <input
              type="date"
              value={before}
              onChange={(e) => setBefore(e.target.value)}
              className="rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-secondary)] px-2 py-1.5 text-sm text-[var(--color-text-primary)]"
            />
          </label>
          {REASON_FILTERS.map((f) => (
            <button
              key={f}
              onClick={() => setReasonFilter(f)}
              className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
                reasonFilter === f
                  ? "bg-[var(--color-accent)] text-white"
                  : "border border-[var(--color-border)] text-[var(--color-text-secondary)] hover:bg-[var(--color-bg-tertiary)]"
              }`}
            >
              {f}
            </button>
          ))}
        </div>
      )}

      {/* Events Grid */}
      {isRecent ? (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {sessions.map((s) => (
            <a
              key={s.session}
              href={`/videos/${activeTab}/${s.session}`}
              className="group relative overflow-hidden rounded bg-[var(--color-bg-card)] shadow-sm transition-all hover:shadow-md"
              style={{ contentVisibility: "auto", containIntrinsicSize: "280px" }}
            >
              <div className="aspect-video bg-[var(--color-bg-tertiary)]">
                {thumbFailed.has(s.session) ? (
                  <ThumbPlaceholder />
                ) : (
                  <img
                    src={sessionThumbnailURL(activeTab, s.session)}
                    alt={s.session}
                    className="h-full w-full object-cover"
                    loading="lazy"
                    onError={() => markThumbFailed(s.session)}
                  />
                )}
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
            </a>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {visibleEvents.map((ev) => (
            <div
              key={ev.name}
              className="group relative overflow-hidden rounded bg-[var(--color-bg-card)] shadow-sm transition-all hover:shadow-md"
              style={{ contentVisibility: "auto", containIntrinsicSize: "280px" }}
            >
              <a href={`/videos/${activeTab}/${ev.name}`} className="block">
                <div className="aspect-video bg-[var(--color-bg-tertiary)]">
                  {thumbFailed.has(ev.name) ? (
                    <ThumbPlaceholder />
                  ) : (
                    <img
                      src={api.thumbnailURL(activeTab, ev.name)}
                      alt={ev.name}
                      className="h-full w-full object-cover"
                      loading="lazy"
                      onError={() => markThumbFailed(ev.name)}
                    />
                  )}
                </div>
                <div className="p-4">
                  <div className="flex items-start justify-between">
                    <p className="text-sm font-semibold text-[var(--color-text-primary)]">
                      {ev.name}
                    </p>
                    {ev.reason && (
                      <span
                        className="ml-2 shrink-0 rounded bg-[var(--color-accent-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-accent-text)]"
                        title={ev.reason}
                      >
                        {formatEventReason(ev.reason)}
                      </span>
                    )}
                  </div>
                  <div className="mt-1 flex items-center gap-3 text-xs text-[var(--color-text-muted)]">
                    <span>{formatDate(ev.datetime)}</span>
                    <span>{formatMegabytes(ev.size_mb)}</span>
                  </div>
                  {ev.city && (
                    <p className="mt-1 text-xs text-[var(--color-text-muted)]">{ev.city}</p>
                  )}
                </div>
              </a>

              {/* Pinned events survive cleanup; the badge shows outside edit mode too */}
              {ev.kept && !editMode && (
                <span
                  className="absolute left-2 top-2 rounded bg-black/60 p-1.5 text-[var(--color-accent)] backdrop-blur-sm"
                  title="Pinned — cleanup will skip this event"
                >
                  <StarIcon filled />
                </span>
              )}

              {editMode && (
                <label className="absolute left-2 top-2 flex items-center gap-2 rounded bg-black/60 px-2 py-1.5 backdrop-blur-sm">
                  <input
                    type="checkbox"
                    checked={selected.has(ev.name)}
                    onChange={() => toggleSelected(ev.name)}
                    className="h-4 w-4 accent-[var(--color-accent)]"
                  />
                </label>
              )}

              <div className="absolute right-2 top-2 flex gap-1">
                {editMode && (
                  <button
                    onClick={() => toggleKeep(ev.name)}
                    className={`rounded bg-black/60 p-1.5 backdrop-blur-sm transition-colors hover:bg-black/80 ${
                      ev.kept ? "text-[var(--color-accent)]" : "text-white"
                    }`}
                    title={ev.kept ? "Unpin from cleanup" : "Pin against cleanup"}
                  >
                    <StarIcon filled={!!ev.kept} />
                  </button>
                )}
                {/* Downloading is a read, so it stays available in Present mode */}
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
                {editMode && (
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
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {loadingEvents && events.length === 0 && sessions.length === 0 && (
        <div className="flex justify-center py-12">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
        </div>
      )}

      {loadError && !loadingEvents && (
        <div className="flex flex-col items-center gap-3 py-12 text-center">
          <p className="text-sm text-[var(--color-danger)]">
            Couldn&apos;t reach the device — this is not an empty folder.
          </p>
          <button
            onClick={() => {
              setLoadError(false);
              if (folders.length) loadEvents(activeTab, 0);
              else {
                setLoading(true);
                loadFolders();
              }
            }}
            className="rounded-sm bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
          >
            Retry
          </button>
        </div>
      )}

      {!loadError &&
        visibleEvents.length === 0 &&
        sessions.length === 0 &&
        !loadingEvents && (
          <div className="py-12 text-center text-sm text-[var(--color-text-muted)]">
            {/* Archive folders live on the Pi, not the TeslaCam drive, so an
                empty one says nothing about the mount — claiming otherwise
                sends the user hunting a drive fault that isn't there. */}
            {teslacamPath === "" && !activeTab.startsWith("archive/")
              ? "TeslaCam drive not mounted — nothing to list until it is."
              : events.length > 0
                ? "No events match this filter"
                : `No events found in ${activeTab}`}
          </div>
        )}

      <div ref={sentinelRef} className="h-4" />

      {loadingMore && (
        <div className="flex justify-center py-4">
          <div className="h-6 w-6 animate-spin rounded-full border-4 border-[var(--color-border)] border-t-[var(--color-accent)]" />
        </div>
      )}

      {editMode && selected.size > 0 && (
        <div className="sticky bottom-4 flex justify-center">
          <button
            onClick={deleteSelected}
            disabled={bulkBusy}
            className="rounded-sm bg-[var(--color-danger)] px-4 py-3 text-sm font-medium text-white shadow-lg disabled:opacity-50"
          >
            {bulkBusy ? "Deleting…" : `Delete selected (${selected.size})`}
          </button>
        </div>
      )}
    </div>
  );
}

// Keeps the card's shape when a thumbnail 404s — hiding the <img> collapsed it.
function ThumbPlaceholder() {
  return (
    <div className="flex h-full w-full items-center justify-center text-[var(--color-text-muted)]">
      <svg className="h-8 w-8" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M6.827 6.175A2.31 2.31 0 015.186 7.23c-.38.054-.757.112-1.134.175C2.999 7.58 2.25 8.507 2.25 9.574V18a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9.574c0-1.067-.75-1.994-1.802-2.169a47.865 47.865 0 00-1.134-.175 2.31 2.31 0 01-1.64-1.055l-.822-1.316a2.192 2.192 0 00-1.736-1.039 48.774 48.774 0 00-5.232 0 2.192 2.192 0 00-1.736 1.039l-.822 1.316z" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M16.5 12.75a4.5 4.5 0 11-9 0 4.5 4.5 0 019 0z" />
      </svg>
    </div>
  );
}

function StarIcon({ filled }: { filled: boolean }) {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 24 24"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth={2}
    >
      <path strokeLinecap="round" strokeLinejoin="round" d="M11.48 3.5l2.2 4.46 4.92.72-3.56 3.47.84 4.9-4.4-2.31-4.4 2.31.84-4.9L4.36 8.68l4.92-.72 2.2-4.46z" />
    </svg>
  );
}
