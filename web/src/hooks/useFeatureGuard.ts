"use client";

import { useEffect, useState } from "react";
import { getStatus } from "@/lib/api";
import type { Features } from "@/lib/types";

type FeatureKey = keyof Features;

interface FeatureGuardResult {
  available: boolean;
  loading: boolean;
}

/**
 * Checks whether a given feature is available according to /api/status.
 * Returns { available, loading } — components should render a disabled state
 * when available=false and loading=false.
 */
export function useFeatureGuard(featureKey: FeatureKey): FeatureGuardResult {
  const [available, setAvailable] = useState(true);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    getStatus()
      .then((status) => {
        if (!cancelled) {
          setAvailable(status.features[featureKey] ?? false);
          setLoading(false);
        }
      })
      .catch(() => {
        if (!cancelled) {
          // On error, default to available so we don't falsely block
          setAvailable(true);
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [featureKey]);

  return { available, loading };
}
