"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import * as api from "@/lib/api";

export type SpeedUnit = "mph" | "kph";

const STORAGE_KEY = "speedUnit";
const DEFAULT_UNIT: SpeedUnit = "kph";

// Server-persisted preference, cached in localStorage for instant UI on first paint.
// Source of truth is /api/config viewer_prefs.speed_unit.
export function useSpeedUnit(): [SpeedUnit, (u: SpeedUnit) => void] {
  const [unit, setUnit] = useState<SpeedUnit>(DEFAULT_UNIT);
  const userSet = useRef(false);

  useEffect(() => {
    let cancelled = false;

    try {
      const cached = localStorage.getItem(STORAGE_KEY);
      if (cached === "mph" || cached === "kph") setUnit(cached);
    } catch {}

    // Always fetch the authoritative value from the server. If it differs from
    // the cached value, update both state and localStorage.
    api
      .getConfig()
      .then((cfg) => {
        // Bail if unmounted or the user already picked a unit — don't clobber
        // their selection with a stale in-flight server value.
        if (cancelled || userSet.current) return;
        const serverUnit = cfg.viewer_prefs?.speed_unit;
        if (serverUnit === "mph" || serverUnit === "kph") {
          setUnit(serverUnit);
          try { localStorage.setItem(STORAGE_KEY, serverUnit); } catch {}
        }
      })
      .catch(() => {
        // Server unreachable — keep cached/default value.
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const set = useCallback((u: SpeedUnit) => {
    userSet.current = true;
    setUnit(u);
    try { localStorage.setItem(STORAGE_KEY, u); } catch {}
    // Persist server-side; ignore failures (cache will retry on next mount).
    api.patchConfig({ viewer_prefs: { speed_unit: u } }).catch(() => {});
  }, []);

  return [unit, set];
}

export function mpsToUnit(mps: number, unit: SpeedUnit): number {
  return Math.round(Math.abs(mps) * (unit === "mph" ? 2.23694 : 3.6));
}
