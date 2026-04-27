"use client";

import { useCallback, useEffect, useState } from "react";

export type SpeedUnit = "mph" | "kph";

const STORAGE_KEY = "speedUnit";

export function useSpeedUnit(): [SpeedUnit, (u: SpeedUnit) => void] {
  const [unit, setUnit] = useState<SpeedUnit>("mph");

  useEffect(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved === "mph" || saved === "kph") setUnit(saved);
    } catch {}
  }, []);

  const set = useCallback((u: SpeedUnit) => {
    setUnit(u);
    try { localStorage.setItem(STORAGE_KEY, u); } catch {}
  }, []);

  return [unit, set];
}

export function mpsToUnit(mps: number, unit: SpeedUnit): number {
  return Math.round(Math.abs(mps) * (unit === "mph" ? 2.23694 : 3.6));
}
