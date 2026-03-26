"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import * as api from "@/lib/api";
import type { LogLine, LogPriority } from "@/lib/types";

const MAX_LINES = 1000;

const PRIORITY_STYLES: Record<LogPriority, string> = {
  error: "text-red-400",
  warn:  "text-amber-400",
  info:  "text-[var(--color-text-primary)]",
  debug: "text-[var(--color-text-muted)]",
};

const PRIORITY_BADGE: Record<LogPriority, string> = {
  error: "bg-red-500/15 text-red-400",
  warn:  "bg-amber-500/15 text-amber-400",
  info:  "bg-[var(--color-accent)]/10 text-[var(--color-accent)]",
  debug: "bg-[var(--color-bg-tertiary)] text-[var(--color-text-muted)]",
};

const UNITS = ["argus", "NetworkManager", "hostapd", "smbd", "sshd", "kernel"];

export default function LogsPage() {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [connected, setConnected] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [filter, setFilter] = useState<LogPriority | "all">("all");
  const [search, setSearch] = useState("");
  const [unit, setUnit] = useState("argus");
  const [follow, setFollow] = useState(true);

  const bottomRef = useRef<HTMLDivElement>(null);
  const esRef = useRef<EventSource | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  const connect = useCallback(() => {
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }
    setLines([]);
    setConnected(false);

    const es = api.openLogsStream({ unit, n: 200, follow });
    esRef.current = es;

    es.onopen = () => setConnected(true);

    es.onmessage = (e) => {
      try {
        const line = JSON.parse(e.data) as LogLine;
        setLines((prev) => {
          const next = [...prev, line];
          return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next;
        });
      } catch {
        // skip malformed lines
      }
    };

    es.onerror = () => {
      setConnected(false);
      es.close();
    };
  }, [unit, follow]);

  useEffect(() => {
    connect();
    return () => {
      esRef.current?.close();
    };
  }, [connect]);

  useEffect(() => {
    if (autoScroll && bottomRef.current) {
      bottomRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [lines, autoScroll]);

  const handleScroll = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60;
    setAutoScroll(atBottom);
  }, []);

  const clearLines = () => setLines([]);

  const visible = lines.filter((l) => {
    if (filter !== "all" && l.priority !== filter) return false;
    if (search && !l.message.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  return (
    <div className="flex h-full flex-col p-6 gap-4">
      <div>
        <h1 className="text-2xl font-bold text-[var(--color-text-primary)]">Journal Logs</h1>
        <p className="mt-1 text-sm text-[var(--color-text-muted)]">
          Live stream of systemd journal entries.
        </p>
      </div>

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3">
        {/* Unit selector */}
        <select
          value={unit}
          onChange={(e) => setUnit(e.target.value)}
          className="rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-3 py-2 text-sm text-[var(--color-text-primary)] focus:border-[var(--color-accent)] focus:outline-none"
        >
          {UNITS.map((u) => (
            <option key={u} value={u}>
              {u}
            </option>
          ))}
        </select>

        {/* Priority filter */}
        <select
          value={filter}
          onChange={(e) => setFilter(e.target.value as LogPriority | "all")}
          className="rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-3 py-2 text-sm text-[var(--color-text-primary)] focus:border-[var(--color-accent)] focus:outline-none"
        >
          <option value="all">All levels</option>
          <option value="error">Error</option>
          <option value="warn">Warning</option>
          <option value="info">Info</option>
          <option value="debug">Debug</option>
        </select>

        {/* Search */}
        <input
          type="search"
          placeholder="Filter messages…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-52 rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-3 py-2 text-sm text-[var(--color-text-primary)] placeholder:text-[var(--color-text-muted)] focus:border-[var(--color-accent)] focus:outline-none"
        />

        {/* Follow toggle */}
        <label className="flex items-center gap-2 text-sm text-[var(--color-text-secondary)]">
          <input
            type="checkbox"
            checked={follow}
            onChange={(e) => setFollow(e.target.checked)}
            className="h-4 w-4 rounded accent-[var(--color-accent)]"
          />
          Follow
        </label>

        {/* Reconnect */}
        <button
          onClick={connect}
          className="rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition-all hover:border-[var(--color-accent)] hover:text-[var(--color-accent-text)]"
        >
          Reconnect
        </button>

        {/* Clear */}
        <button
          onClick={clearLines}
          className="rounded-sm border border-[var(--color-border)] bg-[var(--color-bg-tertiary)] px-4 py-2 text-sm font-medium text-[var(--color-text-secondary)] transition-all hover:border-red-500/50 hover:text-red-400"
        >
          Clear
        </button>

        {/* Status indicator */}
        <div className="ml-auto flex items-center gap-2 text-xs text-[var(--color-text-muted)]">
          <span
            className={`h-2 w-2 rounded-full ${connected ? "bg-emerald-400" : "bg-[var(--color-text-muted)]"}`}
          />
          {connected ? "Connected" : "Disconnected"}
          <span className="ml-2">
            {visible.length}/{lines.length} lines
          </span>
        </div>
      </div>

      {/* Log viewer */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto rounded bg-[var(--color-bg-card)] shadow-sm font-mono text-xs"
        style={{ minHeight: 0 }}
      >
        {visible.length === 0 ? (
          <div className="flex h-32 items-center justify-center text-[var(--color-text-muted)]">
            {connected ? "Waiting for log entries…" : "Not connected."}
          </div>
        ) : (
          <table className="w-full border-collapse">
            <tbody>
              {visible.map((line, i) => (
                <tr
                  key={i}
                  className="border-b border-[var(--color-border)]/30 hover:bg-[var(--color-bg-tertiary)]/50"
                >
                  <td className="w-[180px] whitespace-nowrap px-3 py-1 text-[var(--color-text-muted)]">
                    {line.timestamp}
                  </td>
                  <td className="w-14 px-2 py-1">
                    <span
                      className={`rounded px-1 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${PRIORITY_BADGE[line.priority]}`}
                    >
                      {line.priority}
                    </span>
                  </td>
                  <td
                    className={`px-3 py-1 break-all ${PRIORITY_STYLES[line.priority]}`}
                  >
                    {line.message}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Auto-scroll hint */}
      {!autoScroll && (
        <button
          onClick={() => {
            setAutoScroll(true);
            bottomRef.current?.scrollIntoView({ behavior: "smooth" });
          }}
          className="self-end rounded-sm bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white shadow-lg transition-all hover:bg-[var(--color-accent-hover)]"
        >
          ↓ Scroll to bottom
        </button>
      )}
    </div>
  );
}
