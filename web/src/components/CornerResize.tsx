"use client";

import { useCallback } from "react";

type Corner = "tl" | "tr" | "bl" | "br";

interface CornerResizeProps {
  scale: number;
  onChange: (next: number) => void;
  min: number;
  max: number;
  corner: Corner;
  /** Diagonal pixels of drag that map to one scale unit. Default 220. */
  sensitivity?: number;
}

// Per-corner direction signs (which axis components grow the element when
// dragged outward) and matching CSS resize cursor + Tailwind placement.
const CONFIG: Record<
  Corner,
  { x: number; y: number; cursor: string; pos: string; rotate: number }
> = {
  tl: { x: -1, y: -1, cursor: "nwse-resize", pos: "top-0 left-0", rotate: 180 },
  tr: { x: 1, y: -1, cursor: "nesw-resize", pos: "top-0 right-0", rotate: 270 },
  bl: { x: -1, y: 1, cursor: "nesw-resize", pos: "bottom-0 left-0", rotate: 90 },
  br: { x: 1, y: 1, cursor: "nwse-resize", pos: "bottom-0 right-0", rotate: 0 },
};

// Drag-to-resize affordance for scaled overlays (HUD, Map). Renders a small
// invisible-by-default grip at the requested corner; the parent container is
// expected to expose `group` so the grip fades in on hover. While dragging,
// mouse deltas are converted to a scale offset and clamped to [min, max].
export default function CornerResize({
  scale,
  onChange,
  min,
  max,
  corner,
  sensitivity = 220,
}: CornerResizeProps) {
  const cfg = CONFIG[corner];

  const onMouseDown = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      e.preventDefault();
      e.stopPropagation();

      const startX = e.clientX;
      const startY = e.clientY;
      const startScale = scale;

      const onMove = (ev: MouseEvent) => {
        const dx = ev.clientX - startX;
        const dy = ev.clientY - startY;
        const delta = (cfg.x * dx + cfg.y * dy) / sensitivity;
        const next = Math.min(max, Math.max(min, startScale + delta));
        onChange(next);
      };

      const onUp = () => {
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      };

      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    },
    [scale, onChange, min, max, sensitivity, cfg.x, cfg.y],
  );

  return (
    <div
      onMouseDown={onMouseDown}
      role="slider"
      aria-label="Resize"
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={scale}
      className={`absolute ${cfg.pos} z-30 flex h-4 w-4 items-center justify-center opacity-0 transition-opacity duration-150 group-hover:opacity-90 hover:!opacity-100`}
      style={{ cursor: cfg.cursor }}
      title="Drag to resize"
    >
      <svg
        width="12"
        height="12"
        viewBox="0 0 12 12"
        style={{ transform: `rotate(${cfg.rotate}deg)` }}
      >
        <line x1="11" y1="2" x2="2" y2="11" stroke="white" strokeWidth="1.4" strokeLinecap="round" />
        <line x1="11" y1="6" x2="6" y2="11" stroke="white" strokeWidth="1.4" strokeLinecap="round" />
        <line x1="11" y1="10" x2="10" y2="11" stroke="white" strokeWidth="1.4" strokeLinecap="round" />
      </svg>
    </div>
  );
}
