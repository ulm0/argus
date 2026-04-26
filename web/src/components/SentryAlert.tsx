"use client";

import { useEffect, useState } from "react";
import * as api from "@/lib/api";

interface SentryEvent {
  event_name: string;
  timestamp: string;
  cameras: number;
}

export default function SentryAlert() {
  const [alerts, setAlerts] = useState<SentryEvent[]>([]);

  useEffect(() => {
    const evtSource = new EventSource(api.sentryEventsURL());

    evtSource.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data) as SentryEvent;
        setAlerts((prev) => [...prev, event]);
        setTimeout(() => {
          setAlerts((prev) => prev.filter((a) => a !== event));
        }, 8000);
      } catch {}
    };

    return () => evtSource.close();
  }, []);

  if (alerts.length === 0) return null;

  return (
    <div className="fixed bottom-6 right-6 z-50 flex flex-col gap-2">
      {alerts.map((alert, i) => (
        <div
          key={i}
          className="flex items-start gap-3 rounded-lg bg-[var(--color-bg-card)] border border-[var(--color-border)] px-4 py-3 shadow-lg max-w-xs"
        >
          <div className="mt-0.5 h-2 w-2 shrink-0 rounded-full bg-[var(--color-danger)]" />
          <div className="min-w-0">
            <p className="text-sm font-semibold text-[var(--color-text-primary)]">New Sentry Event</p>
            <p className="mt-0.5 text-xs text-[var(--color-text-muted)] truncate">{alert.event_name}</p>
            <p className="text-xs text-[var(--color-text-muted)]">{alert.cameras} camera{alert.cameras !== 1 ? "s" : ""}</p>
          </div>
          <button
            onClick={() => setAlerts((prev) => prev.filter((_, j) => j !== i))}
            className="ml-auto shrink-0 text-[var(--color-text-muted)] hover:text-[var(--color-text-primary)]"
          >
            ×
          </button>
        </div>
      ))}
    </div>
  );
}
