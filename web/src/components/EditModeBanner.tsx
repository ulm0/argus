"use client";

import { useEffect, useState } from "react";
import * as api from "@/lib/api";
import { useToast } from "@/components/Toast";

// Switching to Edit is destructive to recording, so every entry point warns with
// the same words — a user who learns the consequence in one place must not meet a
// vaguer version of it somewhere else.
export const EDIT_MODE_CONFIRM =
  "Switch to Edit mode? The car will lose the dashcam drive and stop recording until you switch back.";

// The device spends most of its life in Present mode, where the writable mount
// is absent and every upload/delete fails deep in the filesystem layer. Say so
// before the user tries, and offer the one-tap fix.
export default function EditModeBanner() {
  const [mode, setMode] = useState<string | null>(null);
  const [switching, setSwitching] = useState(false);
  const { showToast } = useToast();

  useEffect(() => {
    let alive = true;
    // A failed status call must not nag: the page's own load already reports it.
    api
      .getStatus()
      .then((s) => alive && setMode(s.mode))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  const handleSwitch = async () => {
    if (!confirm(EDIT_MODE_CONFIRM)) return;
    setSwitching(true);
    try {
      await api.switchToEdit();
      const s = await api.getStatus();
      setMode(s.mode);
      showToast("Edit mode active");
    } catch (e) {
      showToast(e instanceof Error ? e.message : "Mode switch failed", false);
    } finally {
      setSwitching(false);
    }
  };

  if (mode === null || mode === "edit") return null;

  return (
    <div className="flex flex-wrap items-center gap-3 rounded-sm border border-[var(--color-warning)] bg-[var(--color-warning-bg)] px-4 py-3">
      <p className="min-w-0 flex-1 text-sm text-[var(--color-text-primary)]">
        Device is in Present mode &mdash; uploads and deletes need Edit mode.
      </p>
      <button
        onClick={handleSwitch}
        disabled={switching}
        className="rounded bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[var(--color-accent-hover)] disabled:opacity-60"
      >
        {switching ? "Switching…" : "Switch to Edit"}
      </button>
    </div>
  );
}
