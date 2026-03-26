"use client";

import { formatBytes } from "./FileSize";

interface StorageBarProps {
  used: number;
  total: number;
  label?: string;
  className?: string;
}

function colorForPercent(pct: number): string {
  if (pct < 50) return "var(--color-success, #25cb55)";
  if (pct < 70) return "var(--color-warning, #ffbd0a)";
  if (pct < 85) return "var(--color-danger, #ed4e3b)";
  return "var(--color-danger, #ed4e3b)";
}

export default function StorageBar({ used, total, label, className = "" }: StorageBarProps) {
  const pct = total > 0 ? Math.min((used / total) * 100, 100) : 0;
  const roundedPct = Math.round(pct * 10) / 10;
  const barColor = colorForPercent(pct);

  return (
    <div className={`w-full ${className}`}>
      {(label || true) && (
        <div className="flex items-center justify-between mb-1.5 text-sm">
          {label && (
            <span className="font-medium text-[var(--color-text-primary)]">{label}</span>
          )}
          <span className="text-[var(--color-text-secondary)] tabular-nums ml-auto">
            {formatBytes(used)} / {formatBytes(total)}
            <span className="ml-1.5 font-semibold" style={{ color: barColor }}>
              {roundedPct}%
            </span>
          </span>
        </div>
      )}
      <div className="relative h-2.5 w-full overflow-hidden rounded-full bg-[var(--color-bg-tertiary)]">
        <div
          className="h-full rounded-full transition-all duration-500 ease-out"
          style={{
            width: `${pct}%`,
            background: `linear-gradient(90deg, var(--color-success, #25cb55) 0%, ${barColor} 100%)`,
          }}
        />
      </div>
    </div>
  );
}
