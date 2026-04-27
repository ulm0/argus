"use client";

import { memo } from "react";
import { AutopilotState, GearState } from "@/lib/sei-parser";
import type { SeiMetadata } from "@/lib/sei-parser";
import { useSpeedUnit, mpsToUnit } from "@/lib/useSpeedUnit";

interface ArgusHUDProps {
  sei: SeiMetadata | null;
  visible: boolean;
}

const GEAR_LABELS: Record<GearState, string> = {
  [GearState.PARK]: "P",
  [GearState.DRIVE]: "D",
  [GearState.REVERSE]: "R",
  [GearState.NEUTRAL]: "N",
};

const AP_LABELS: Record<AutopilotState, string> = {
  [AutopilotState.NONE]: "",
  [AutopilotState.SELF_DRIVING]: "Full Self-Driving",
  [AutopilotState.AUTOSTEER]: "Autosteer",
  [AutopilotState.TACC]: "Traffic-Aware Cruise",
};

function ArgusHUD({ sei, visible }: ArgusHUDProps) {
  const [speedUnit] = useSpeedUnit();

  if (!visible) return null;

  const speed = sei ? mpsToUnit(sei.vehicleSpeedMps, speedUnit) : 0;
  const gear = sei ? (GEAR_LABELS[sei.gearState] ?? "P") : "P";
  const wheelAngle = sei?.steeringWheelAngle ?? 0;
  const leftBlinker = sei?.blinkerOnLeft ?? false;
  const rightBlinker = sei?.blinkerOnRight ?? false;
  const apLabel = sei ? (AP_LABELS[sei.autopilotState] ?? "") : "";
  const isAutopilot = sei ? sei.autopilotState !== AutopilotState.NONE : false;

  return (
    <div
      className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex items-center justify-between px-3 py-2"
      style={{
        background: "linear-gradient(to top, rgba(0,0,0,0.85) 0%, rgba(0,0,0,0.4) 100%)",
      }}
    >
      {/* Gear badge */}
      <div
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-extrabold"
        style={{
          background: isAutopilot ? "rgba(50,195,255,0.22)" : "rgba(255,255,255,0.12)",
          border: `1.5px solid ${isAutopilot ? "rgba(50,195,255,0.45)" : "rgba(255,255,255,0.2)"}`,
          color: isAutopilot ? "#4fc3f7" : "#fff",
        }}
      >
        {gear}
      </div>

      {/* Center: left blinker · speed · AP label · right blinker */}
      <div className="flex flex-1 items-center justify-center gap-3">
        <BlinkerChevron side="left" active={leftBlinker} />

        <div className="flex items-baseline gap-2">
          <span
            className="font-extrabold text-white tabular-nums leading-none"
            style={{ fontSize: "clamp(16px, 2.2vw, 24px)" }}
          >
            {speed}
          </span>
          <span className="text-[11px] font-semibold text-white/55 leading-none self-end pb-px">
            {speedUnit.toUpperCase()}
          </span>
          {apLabel && (
            <>
              <span className="text-white/25 select-none self-center text-xs">·</span>
              <span
                className="font-semibold leading-none self-center"
                style={{ fontSize: "clamp(11px, 1.2vw, 13px)", color: "#4fc3f7" }}
              >
                {apLabel}
              </span>
            </>
          )}
        </div>

        <BlinkerChevron side="right" active={rightBlinker} />
      </div>

      {/* Steering wheel */}
      <div
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full"
        style={{
          background: "rgba(255,255,255,0.08)",
          border: "1.5px solid rgba(255,255,255,0.16)",
        }}
      >
        <svg
          width="21"
          height="21"
          viewBox="0 0 24 24"
          fill="none"
          stroke="rgba(255,255,255,0.82)"
          strokeWidth={1.75}
          strokeLinecap="round"
          style={{ transform: `rotate(${wheelAngle}deg)`, transition: "transform 0.08s linear" }}
        >
          <circle cx="12" cy="12" r="9" />
          <circle cx="12" cy="12" r="2.5" />
          <line x1="12" y1="3" x2="12" y2="9.5" />
          <line x1="3" y1="12" x2="9.5" y2="12" />
          <line x1="14.5" y1="12" x2="21" y2="12" />
        </svg>
      </div>
    </div>
  );
}

export default memo(ArgusHUD);

function BlinkerChevron({ side, active }: { side: "left" | "right"; active: boolean }) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke={active ? "#4ade80" : "rgba(255,255,255,0.2)"}
      strokeWidth={2.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{
        filter: active ? "drop-shadow(0 0 5px rgba(74,222,128,0.65))" : "none",
        transition: "stroke 0.12s ease, filter 0.12s ease",
      }}
    >
      {side === "left" ? (
        <path d="M15 18l-6-6 6-6" />
      ) : (
        <path d="M9 18l6-6-6-6" />
      )}
    </svg>
  );
}
