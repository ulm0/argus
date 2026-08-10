"use client";

import { useCallback, useRef } from "react";

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
// grip at the requested corner; the parent container is expected to expose
// `group` so the grip fades in on hover (it stays visible on touch, where
// there is no hover to reveal it). While dragging, pointer deltas are
// converted to a scale offset and clamped to [min, max].
export default function CornerResize({
  scale,
  onChange,
  min,
  max,
  corner,
  sensitivity = 220,
}: CornerResizeProps) {
  const cfg = CONFIG[corner];
  const dragRef = useRef<{ x: number; y: number; scale: number } | null>(null);

  const onPointerDown = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      e.preventDefault();
      e.stopPropagation();
      dragRef.current = { x: e.clientX, y: e.clientY, scale };
      e.currentTarget.setPointerCapture(e.pointerId);
    },
    [scale],
  );

  const onPointerMove = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      const start = dragRef.current;
      if (!start) return;
      const delta =
        (cfg.x * (e.clientX - start.x) + cfg.y * (e.clientY - start.y)) / sensitivity;
      onChange(Math.min(max, Math.max(min, start.scale + delta)));
    },
    [onChange, min, max, sensitivity, cfg.x, cfg.y],
  );

  const onPointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    dragRef.current = null;
    e.currentTarget.releasePointerCapture(e.pointerId);
  }, []);

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const step = e.key === "ArrowUp" ? 0.1 : e.key === "ArrowDown" ? -0.1 : 0;
      if (!step) return;
      e.preventDefault();
      onChange(Math.min(max, Math.max(min, scale + step)));
    },
    [scale, onChange, min, max],
  );

  return (
    <div
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onKeyDown={onKeyDown}
      role="slider"
      tabIndex={0}
      aria-label="Resize"
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={scale}
      className={`absolute ${cfg.pos} z-30 flex h-4 w-4 touch-none items-center justify-center opacity-0 transition-opacity duration-150 group-hover:opacity-90 hover:!opacity-100 [@media(pointer:coarse)]:opacity-90`}
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
