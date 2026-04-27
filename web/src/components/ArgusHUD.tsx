"use client";

import { memo } from "react";
import { AutopilotState, GearState } from "@/lib/sei-parser";
import type { SeiMetadata } from "@/lib/sei-parser";
import { useSpeedUnit, mpsToUnit } from "@/lib/useSpeedUnit";

interface ArgusHUDProps {
  sei: SeiMetadata | null;
  visible: boolean;
  scale?: number;
  onScaleWheel?: (e: React.WheelEvent<HTMLDivElement>) => void;
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

function ArgusHUD({ sei, visible, scale = 1, onScaleWheel }: ArgusHUDProps) {
  const [speedUnit] = useSpeedUnit();

  if (!visible) return null;

  const speed = sei ? mpsToUnit(sei.vehicleSpeedMps, speedUnit) : 0;
  const gear = sei ? GEAR_LABELS[sei.gearState] ?? "P" : "P";
  const wheelAngle = sei ? sei.steeringWheelAngle : 0;
  const leftBlinker = sei?.blinkerOnLeft ?? false;
  const rightBlinker = sei?.blinkerOnRight ?? false;
  const brakeOn = sei?.brakeApplied ?? false;
  const throttleOn = (sei?.acceleratorPedalPosition ?? 0) > 0.02;
  const apState = sei?.autopilotState ?? AutopilotState.NONE;
  const apLabel = AP_LABELS[apState] || "";

  return (
    <div
      className="absolute inset-x-0 top-3 z-20 flex justify-center"
      style={{ transformOrigin: "top center", transform: `scale(${scale})` }}
      onWheel={onScaleWheel}
      title={`HUD size: ${scale.toFixed(2)}x (scroll to resize)`}
    >
      <div
        className="border border-white/[0.10] shadow-[0_12px_32px_rgba(0,0,0,0.35)]"
        style={{
          width: "230px",
          padding: "6px 10px 8px",
          borderRadius: "14px",
          backdropFilter: "blur(14px)",
          background: "rgba(22, 26, 30, 0.78)",
        }}
      >
        {/* Row 1: corner badges + arrows + speed */}
        <div
          className="grid items-center"
          style={{
            gridTemplateColumns: "20px 14px 1fr 14px 20px",
            columnGap: "6px",
          }}
        >
          {/* Gear pill */}
          <div
            className="flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-bold text-white"
            style={{ background: "rgba(45, 101, 255, 0.85)" }}
          >
            {gear}
          </div>

          {/* Left blinker arrow */}
          <ArrowBlinker side="left" active={leftBlinker} />

          {/* Speed */}
          <div className="flex flex-col items-center leading-none">
            <span
              className="text-[26px] font-bold text-white/95"
              style={{ fontVariantNumeric: "tabular-nums" }}
            >
              {speed}
            </span>
            <span className="mt-0.5 text-[10px] font-semibold tracking-wider text-white/70">
              {speedUnit.toUpperCase()}
            </span>
          </div>

          {/* Right blinker arrow */}
          <ArrowBlinker side="right" active={rightBlinker} />

          {/* Steering wheel */}
          <div
            className="flex h-5 w-5 items-center justify-center rounded-full text-white/70"
            style={{ background: "rgba(255,255,255,0.06)" }}
          >
            <svg
              className="h-3.5 w-3.5"
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

        {/* Row 2: brake icon + autopilot label + info icon */}
        <div
          className="mt-1 grid items-center"
          style={{ gridTemplateColumns: "20px 1fr 20px", columnGap: "6px" }}
        >
          <div
            className="flex h-4 w-4 items-center justify-center"
            style={{ color: brakeOn ? "#ff5a5a" : "rgba(255,255,255,0.4)" }}
          >
            <BrakeIcon />
          </div>
          <div
            className="truncate text-center text-[11px] font-semibold leading-tight"
            style={{ color: "#1f6dff" }}
          >
            {apLabel}
          </div>
          <div
            className="flex h-4 w-4 items-center justify-center"
            style={{ color: throttleOn ? "#25c05a" : "rgba(255,255,255,0.4)" }}
          >
            <ThrottleIcon />
          </div>
        </div>
      </div>
    </div>
  );
}

export default memo(ArgusHUD);

function ArrowBlinker({ side, active }: { side: "left" | "right"; active: boolean }) {
  return (
    <span
      className={active ? "text-[#25c05a]" : "text-white/30"}
      style={{ transition: "color 0.1s ease-out", display: "inline-flex", justifyContent: "center" }}
    >
      <svg className="h-3 w-3" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
        {side === "left" ? <path d="M15 6l-7 6 7 6V6z" /> : <path d="M9 6l7 6-7 6V6z" />}
      </svg>
    </span>
  );
}

function BrakeIcon() {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <rect x="4" y="13" width="16" height="8" rx="2.5" />
      <rect x="9" y="3" width="6" height="10" rx="1.5" />
    </svg>
  );
}

function ThrottleIcon() {
  return (
    <svg
      className="h-4 w-4"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <rect x="10" y="3" width="4" height="6" rx="1.5" />
      <path d="M14 9 L14 14 Q14 16 16 16 L16 21 Q16 21 14 21 L10 21 Q8 21 8 19 L8 16" />
    </svg>
  );
}
