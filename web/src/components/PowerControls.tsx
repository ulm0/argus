"use client";

import { useState } from "react";
import * as api from "@/lib/api";

const btnCls =
  "flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg-card)] px-3 py-2 text-xs font-semibold text-[var(--color-text-secondary)] transition-all disabled:opacity-50";

// PowerControls is rendered both in SystemPanel (xl and up) and in the dashboard's
// xl:hidden card, so a phone can sign out and reboot a wedged gadget without
// having to pull the plug.
export default function PowerControls() {
  const [powerAction, setPowerAction] = useState<"reboot" | "poweroff" | null>(null);

  const handleLogout = async () => {
    try {
      await api.logout();
    } catch {
      /* ignore — we reload regardless */
    }
    // Reload so AuthGate re-evaluates auth status (shows login if enabled).
    window.location.reload();
  };

  const handlePower = async (action: "reboot" | "poweroff") => {
    const msg =
      action === "reboot"
        ? "Reboot the device?"
        : "Shut down Argus? Dashcam/Sentry recording stops and the device stays OFF until power is physically reconnected.";
    if (!confirm(msg)) return;
    setPowerAction(action);
    try {
      if (action === "reboot") await api.rebootSystem();
      else await api.powerOffSystem();
    } catch {
      // The request will be cut off as the host goes down — that's expected.
      // Anything truly fatal will surface when the user reloads the page.
    }
  };

  return (
    <>
      <div className="mt-3 flex gap-2">
        <button
          onClick={() => handlePower("reboot")}
          disabled={powerAction !== null}
          className={`${btnCls} hover:border-[var(--color-warning)] hover:text-[var(--color-warning)]`}
        >
          {powerAction === "reboot" ? "Rebooting..." : "Restart"}
        </button>
        <button
          onClick={() => handlePower("poweroff")}
          disabled={powerAction !== null}
          className={`${btnCls} hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]`}
        >
          {powerAction === "poweroff" ? "Shutting down..." : "Shut down"}
        </button>
      </div>
      <button
        onClick={handleLogout}
        className={`${btnCls} mt-2 w-full hover:border-[var(--color-text-primary)] hover:text-[var(--color-text-primary)]`}
      >
        Sign out
      </button>
    </>
  );
}
