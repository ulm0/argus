// Human-readable byte formatting used across the /videos pages.
// Picks the largest unit that keeps the number ≥ 1 (e.g. 1586.7 MB → 1.5 GB,
// 950 MB stays as 950.0 MB) so users don't have to mentally divide by 1024.

const UNITS = ["B", "KB", "MB", "GB", "TB"] as const;
const STEP = 1024;

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const i = Math.min(
    UNITS.length - 1,
    Math.floor(Math.log(bytes) / Math.log(STEP)),
  );
  // Show one decimal of precision, but drop a trailing ".0" so whole-unit
  // values render as "950 MB" rather than "950.0 MB".
  const value = (bytes / Math.pow(STEP, i)).toFixed(1);
  const display = value.endsWith(".0") ? value.slice(0, -2) : value;
  return `${display} ${UNITS[i]}`;
}

// Convenience for inputs already expressed in megabytes (Tesla event metadata
// reports `size_mb`); keeps callers from doing `* 1024 * 1024` inline.
export function formatMegabytes(mb: number): string {
  return formatBytes(mb * STEP * STEP);
}
