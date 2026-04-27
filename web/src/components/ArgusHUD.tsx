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
  const apState = sei?.autopilotState ?? AutopilotState.NONE;
  const apLabel = AP_LABELS[apState] ?? "—";

  return (
    <div
      className="absolute inset-x-0 top-3 z-20 flex justify-center"
      style={{ transformOrigin: "top center", transform: `scale(${scale})` }}
      onWheel={onScaleWheel}
      title={`HUD size: ${scale.toFixed(2)}x (scroll to resize)`}
    >
      <div
        className="border border-white/[0.14] shadow-[0_12px_32px_rgba(0,0,0,0.35)]"
        style={{
          width: "260px",
          minHeight: "108px",
          padding: "10px 12px 8px",
          borderRadius: "14px",
          backdropFilter: "blur(14px)",
          background: "rgba(20, 22, 24, 0.72)",
        }}
      >
        <div
          className="grid items-center"
          style={{
            gridTemplateColumns: "24px 1fr 24px",
            gridTemplateRows: "20px auto 20px",
            gridTemplateAreas:
              '"leftTop topRow rightTop" "leftMid center rightMid" "leftBottom autopilot rightBottom"',
            columnGap: "10px",
            rowGap: "4px",
          }}
        >
          <div style={{ gridArea: "leftTop" }} className="flex h-5 w-5 items-center justify-center rounded-full bg-[#2d65ff]/25 text-[10px] font-bold text-[#6da1ff]">
            {gear}
          </div>

          <div style={{ gridArea: "leftMid" }} className="flex items-center justify-center text-white/55">
            <SmallDotIcon />
          </div>

          <div style={{ gridArea: "leftBottom" }} />

          <div
            style={{ gridArea: "topRow" }}
            className="flex items-center justify-center gap-6 pt-0.5"
          >
            <ArrowBlinker side="left" active={leftBlinker} />
            <ArrowBlinker side="right" active={rightBlinker} />
          </div>

          <div style={{ gridArea: "center" }} className="flex flex-col items-center leading-none">
            <span className="text-[42px] font-bold text-white/95" style={{ fontVariantNumeric: "tabular-nums" }}>
              {speed}
            </span>
            <span className="mt-0.5 text-[11px] font-semibold tracking-wide text-white/75">{speedUnit.toUpperCase()}</span>
          </div>

          <div style={{ gridArea: "autopilot" }} className="truncate text-center text-[11px] font-semibold text-[#1f6dff]">
            {apLabel}
          </div>

          <div
            style={{ gridArea: "rightTop" }}
            className="flex h-5 w-5 items-center justify-center text-white/55"
          >
            <svg
              className="h-4 w-4"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={1.5}
              style={{
                transform: `rotate(${wheelAngle}deg)`,
                transition: "transform 0.1s ease-out",
              }}
            >
              <circle cx="12" cy="12" r="9" />
              <circle cx="12" cy="12" r="3" />
              <line x1="12" y1="3" x2="12" y2="9" />
              <line x1="3" y1="12" x2="9" y2="12" />
              <line x1="15" y1="12" x2="21" y2="12" />
            </svg>
          </div>

          <div style={{ gridArea: "rightMid" }} />

          <div style={{ gridArea: "rightBottom" }} className="flex h-5 w-5 items-center justify-center text-white/55">
            <InfoIcon />
          </div>
        </div>
      </div>
    </div>
  );
}

export default memo(ArgusHUD);

function ArrowBlinker({ side, active }: { side: "left" | "right"; active: boolean }) {
  return (
    <div
      className={`flex items-center justify-center ${active ? "text-[#25c05a]" : "text-white/35"}`}
    >
      <svg
        className="h-4 w-4"
        viewBox="0 0 24 24"
        fill="currentColor"
      >
        {side === "left" ? (
          <path d="M14 7l-5 5 5 5V7z" />
        ) : (
          <path d="M10 17l5-5-5-5v10z" />
        )}
      </svg>
    </div>
  );
}

function SmallDotIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <circle cx="12" cy="12" r="8" />
    </svg>
  );
}

function InfoIcon() {
  return (
    <svg className="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.7}>
      <circle cx="12" cy="12" r="9" />
      <line x1="12" y1="10" x2="12" y2="16" />
      <circle cx="12" cy="7.2" r="1" fill="currentColor" stroke="none" />
    </svg>
  );
}
