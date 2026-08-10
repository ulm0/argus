"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import * as api from "@/lib/api";

interface SentryEvent {
  event_name: string;
  timestamp: string;
  cameras: number;
}

const keyOf = (e: SentryEvent) => `${e.timestamp}-${e.event_name}`;

export default function SentryAlert() {
  const [alerts, setAlerts] = useState<SentryEvent[]>([]);
  const [disconnected, setDisconnected] = useState(false);

  useEffect(() => {
    const evtSource = new EventSource(api.sentryEventsURL());

    evtSource.onopen = () => setDisconnected(false);
    evtSource.onerror = () => setDisconnected(true);

    evtSource.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data) as SentryEvent;
        // Sentry alerts never auto-dismiss: this is the incident-response moment
        // and the phone is usually locked when the event fires.
        setAlerts((prev) =>
          prev.some((a) => keyOf(a) === keyOf(event)) ? prev : [...prev, event],
        );
      } catch {}
    };

    return () => evtSource.close();
  }, []);

  if (alerts.length === 0 && !disconnected) return null;

  return (
    <div
      role="alert"
      className="fixed bottom-6 right-6 z-50 flex max-w-xs flex-col gap-2"
    >
      {alerts.map((alert) => (
        <div
          key={keyOf(alert)}
          className="flex items-start gap-3 rounded-lg bg-[var(--color-bg-card)] border border-[var(--color-border)] px-4 py-3 shadow-lg"
        >
          <div className="mt-0.5 h-2 w-2 shrink-0 rounded-full bg-[var(--color-danger)]" />
          <Link href="/videos/SentryClips" className="min-w-0">
            <p className="text-sm font-semibold text-[var(--color-text-primary)]">New Sentry Event</p>
            <p className="mt-0.5 text-xs text-[var(--color-text-muted)] truncate">{alert.event_name}</p>
            <p className="text-xs text-[var(--color-text-muted)]">{alert.cameras} camera{alert.cameras !== 1 ? "s" : ""}</p>
          </Link>
          <button
            onClick={() => setAlerts((prev) => prev.filter((a) => keyOf(a) !== keyOf(alert)))}
            aria-label="Dismiss"
            className="ml-auto shrink-0 text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)]"
          >
            ×
          </button>
        </div>
      ))}
      {disconnected && (
        <p className="rounded-lg bg-[var(--color-bg-card)] border border-[var(--color-border)] px-4 py-2 text-xs text-[var(--color-text-muted)] shadow-lg">
          Sentry alerts disconnected — retrying
        </p>
      )}
    </div>
  );
}
