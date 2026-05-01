"use client";

import { memo } from "react";
import {
  PiArrowFatLeftFill,
  PiArrowFatLeftLight,
  PiArrowFatRightFill,
  PiArrowFatRightLight,
} from "react-icons/pi";
import { AutopilotState, GearState } from "@/lib/sei-parser";
import type { SeiMetadata } from "@/lib/sei-parser";
import { useSpeedUnit, mpsToUnit } from "@/lib/useSpeedUnit";
import CornerResize from "./CornerResize";

// Allowed scale range for the HUD overlay; kept in sync with the wheel-based
// resize logic in DashcamPlayer.
const HUD_SCALE_MIN = 0.7;
const HUD_SCALE_MAX = 1.8;

interface ArgusHUDProps {
  sei: SeiMetadata | null;
  visible: boolean;
  scale?: number;
  onScaleWheel?: (e: React.WheelEvent<HTMLDivElement>) => void;
  onScaleChange?: (next: number) => void;
}

const GEAR_LABELS: Record<GearState, string> = {
  [GearState.PARK]: "P",
  [GearState.DRIVE]: "D",
  [GearState.REVERSE]: "R",
  [GearState.NEUTRAL]: "N",
};

const AP_LABELS: Record<AutopilotState, string> = {
  [AutopilotState.NONE]: "",
  [AutopilotState.SELF_DRIVING]: "Self Driving",
  [AutopilotState.AUTOSTEER]: "Autosteer",
  [AutopilotState.TACC]: "Cruise",
};

function ArgusHUD({ sei, visible, scale = 1, onScaleWheel, onScaleChange }: ArgusHUDProps) {
  const [speedUnit] = useSpeedUnit();

  if (!visible) return null;

  const speed = sei ? mpsToUnit(sei.vehicleSpeedMps, speedUnit) : 0;
  const gear = sei ? GEAR_LABELS[sei.gearState] ?? "P" : "P";
  const wheelAngle = sei ? sei.steeringWheelAngle : 0;
  const leftBlinker = sei?.blinkerOnLeft ?? false;
  const rightBlinker = sei?.blinkerOnRight ?? false;
  const brakeOn = sei?.brakeApplied ?? false;
  // acceleratorPedalPosition can come as 0..1 (fraction) or 0..100 (percent)
  const rawThrottle = sei?.acceleratorPedalPosition ?? 0;
  const throttlePct = Math.max(
    0,
    Math.min(100, rawThrottle <= 1.2 ? rawThrottle * 100 : rawThrottle),
  );
  const apState = sei?.autopilotState ?? AutopilotState.NONE;
  const apLabel = AP_LABELS[apState] || "";

  return (
    <div
      className="absolute inset-x-0 top-3 z-20 flex justify-center"
      style={{ transformOrigin: "top center", transform: `scale(${scale})` }}
      onWheel={onScaleWheel}
      title={`HUD size: ${scale.toFixed(2)}x (scroll or drag corner to resize)`}
    >
      <div
        className="group relative border border-white/[0.10] shadow-[0_12px_32px_rgba(0,0,0,0.35)]"
        style={{
          width: "320px",
          padding: "10px 14px 12px",
          borderRadius: "18px",
          backdropFilter: "blur(14px)",
          background: "rgba(22, 26, 30, 0.78)",
        }}
      >
        {onScaleChange && (
          <CornerResize
            scale={scale}
            onChange={onScaleChange}
            min={HUD_SCALE_MIN}
            max={HUD_SCALE_MAX}
            corner="br"
          />
        )}
        {/* Row 1: corner badges + arrows + speed */}
        <div
          className="grid items-center"
          style={{
            gridTemplateColumns: "32px 22px 1fr 22px 32px",
            columnGap: "8px",
          }}
        >
          {/* Gear pill */}
          <div
            className="flex h-7 w-7 items-center justify-center rounded-full text-[12px] font-bold text-white"
            style={{ background: "rgba(45, 101, 255, 0.85)" }}
          >
            {gear}
          </div>

          {/* Left blinker arrow */}
          <ArrowBlinker side="left" active={leftBlinker} />

          {/* Speed */}
          <div className="flex flex-col items-center leading-none">
            <span
              className="text-[36px] font-bold text-white/95"
              style={{ fontVariantNumeric: "tabular-nums", lineHeight: "1" }}
            >
              {speed}
            </span>
            <span className="mt-1 text-[11px] font-semibold tracking-wider text-white/70">
              {speedUnit.toUpperCase()}
            </span>
          </div>

          {/* Right blinker arrow */}
          <ArrowBlinker side="right" active={rightBlinker} />

          {/* Steering wheel */}
          <div
            className="flex h-7 w-7 items-center justify-center rounded-full text-white/75"
            style={{ background: "rgba(255,255,255,0.06)" }}
          >
            <svg
              className="h-4 w-4"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={1.6}
              style={{
                transform: `rotate(${wheelAngle}deg)`,
                transition: "transform 0.1s ease-out",
              }}
            >
              <circle cx="12" cy="12" r="9" />
              <circle cx="12" cy="12" r="2.5" />
              <line x1="12" y1="3" x2="12" y2="9.5" />
              <line x1="3" y1="12" x2="9.5" y2="12" />
              <line x1="14.5" y1="12" x2="21" y2="12" />
            </svg>
          </div>
        </div>

        {/* Row 2: brake pedal + autopilot label + throttle pedal */}
        <div
          className="mt-1 grid items-center"
          style={{ gridTemplateColumns: "32px 1fr 32px", columnGap: "8px" }}
        >
          <PedalIndicator
            kind="brake"
            fillPct={brakeOn ? 100 : 0}
            color="rgba(255, 90, 90, 0.85)"
          />
          <div
            className="truncate text-center text-[12px] font-semibold leading-tight"
            style={{ color: "#1f6dff" }}
          >
            {apLabel}
          </div>
          <PedalIndicator
            kind="throttle"
            fillPct={throttlePct}
            color="rgba(120, 220, 120, 0.85)"
          />
        </div>
      </div>
    </div>
  );
}

export default memo(ArgusHUD);

function ArrowBlinker({ side, active }: { side: "left" | "right"; active: boolean }) {
  // Use the filled variant when the blinker is on so it reads as a solid green
  // arrow (matches the in-car cluster behavior); thin outline when idle.
  const Icon = active
    ? side === "left"
      ? PiArrowFatLeftFill
      : PiArrowFatRightFill
    : side === "left"
      ? PiArrowFatLeftLight
      : PiArrowFatRightLight;
  return (
    <span
      className={active ? "text-[#25c05a]" : "text-white/30"}
      style={{
        transition: "color 0.1s ease-out",
        display: "inline-flex",
        justifyContent: "center",
      }}
    >
      <Icon size={22} />
    </span>
  );
}

function PedalIndicator({
  kind,
  fillPct,
  color,
}: {
  kind: "brake" | "throttle";
  fillPct: number;
  color: string;
}) {
  const clamped = Math.min(100, Math.max(0, fillPct));

  return (
    <div
      className="relative flex h-7 w-7 items-center justify-center rounded-full"
      style={{
        background: "rgba(255,255,255,0.05)",
        border: "1px solid rgba(255,255,255,0.10)",
      }}
    >
      {/* Proportional fill from bottom */}
      <div
        className="absolute inset-0 overflow-hidden rounded-full"
        aria-hidden="true"
      >
        <div
          className="absolute inset-x-0 bottom-0"
          style={{
            height: `${clamped}%`,
            background: color,
            transition: "height 0.1s ease-out",
          }}
        />
      </div>
      {/* Pedal icon */}
      <svg
        className="relative z-10 h-4 w-4"
        viewBox="0 0 24 24"
        fill="none"
        stroke="rgba(230, 230, 230, 0.92)"
        strokeWidth={1.6}
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        {kind === "brake" ? (
          <>
            <rect x="4" y="13" width="16" height="8" rx="2.5" />
            <rect x="9" y="3" width="6" height="10" rx="1.5" />
          </>
        ) : (
          <>
            <rect x="10" y="3" width="4" height="6" rx="1.5" />
            <path d="M14 9 L14 14 Q14 16 16 16 L16 21 Q16 21 14 21 L10 21 Q8 21 8 19 L8 16" />
          </>
        )}
      </svg>
    </div>
  );
}
