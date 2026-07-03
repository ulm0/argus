"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import * as api from "@/lib/api";

// Server-persisted overlay scales (HUD, Map). Mirrors the behaviour of
// useSpeedUnit: localStorage is just a cache for instant first paint, the
// authoritative value lives in /api/config viewer_prefs.{hud,map}_scale.
//
// During a drag the setter is hammered at ~60fps; we debounce the server
// PATCH so the YAML config isn't rewritten dozens of times per second.

type Kind = "hud" | "map";

const STORAGE_KEYS: Record<Kind, string> = {
  hud: "hudScale",
  map: "mapScale",
};

const SAVE_DEBOUNCE_MS = 400;

function clamp(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v));
}

interface Options {
  kind: Kind;
  min: number;
  max: number;
  defaultValue?: number;
}

export function useViewerScale({
  kind,
  min,
  max,
  defaultValue = 1.0,
}: Options): [number, (next: number) => void] {
  const [scale, setScale] = useState(defaultValue);
  const debounceRef = useRef<number | null>(null);
  const latestRef = useRef<number>(defaultValue);
  const userSet = useRef(false);

  useEffect(() => {
    let cancelled = false;

    try {
      const cached = localStorage.getItem(STORAGE_KEYS[kind]);
      if (cached) {
        const n = parseFloat(cached);
        if (Number.isFinite(n)) {
          const c = clamp(n, min, max);
          setScale(c);
          latestRef.current = c;
        }
      }
    } catch {}

    api
      .getConfig()
      .then((cfg) => {
        // Bail if the user has already started interacting; a late config
        // response must not snap the overlay back to the stale server value
        // mid-drag while the pending PATCH still saves the drag value.
        if (cancelled || userSet.current) return;
        const v = kind === "hud" ? cfg.viewer_prefs?.hud_scale : cfg.viewer_prefs?.map_scale;
        if (typeof v === "number" && Number.isFinite(v) && v > 0) {
          const c = clamp(v, min, max);
          setScale(c);
          latestRef.current = c;
          try { localStorage.setItem(STORAGE_KEYS[kind], String(c)); } catch {}
        }
      })
      .catch(() => {
        // Server unreachable — keep cached/default value silently.
      });

    return () => {
      cancelled = true;
    };
  }, [kind, min, max]);

  // Flush any pending debounced save when the component unmounts so the user
  // doesn't lose the last fraction of a drag if they navigate away quickly.
  useEffect(() => {
    return () => {
      if (debounceRef.current != null) {
        window.clearTimeout(debounceRef.current);
        const v = latestRef.current;
        const patch = kind === "hud" ? { hud_scale: v } : { map_scale: v };
        api.patchConfig({ viewer_prefs: patch }).catch(() => {});
      }
    };
  }, [kind]);

  const set = useCallback(
    (next: number) => {
      userSet.current = true;
      const c = clamp(next, min, max);
      setScale(c);
      latestRef.current = c;
      try { localStorage.setItem(STORAGE_KEYS[kind], String(c)); } catch {}

      if (debounceRef.current != null) window.clearTimeout(debounceRef.current);
      debounceRef.current = window.setTimeout(() => {
        debounceRef.current = null;
        const patch = kind === "hud" ? { hud_scale: c } : { map_scale: c };
        api.patchConfig({ viewer_prefs: patch }).catch(() => {});
      }, SAVE_DEBOUNCE_MS);
    },
    [kind, min, max],
  );

  return [scale, set];
}
