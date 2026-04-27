"use client";

import { useCallback, useEffect, useState } from "react";

export type MapTheme = "light" | "dark";

const STORAGE_KEY = "mapTheme";

export const TILE_URLS: Record<MapTheme, string> = {
  light: "https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png",
  dark:  "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png",
};

export function useMapTheme(): [MapTheme, (t: MapTheme) => void] {
  const [theme, setTheme] = useState<MapTheme>("light");

  useEffect(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved === "light" || saved === "dark") setTheme(saved);
    } catch {}
  }, []);

  const set = useCallback((t: MapTheme) => {
    setTheme(t);
    try { localStorage.setItem(STORAGE_KEY, t); } catch {}
  }, []);

  return [theme, set];
}
