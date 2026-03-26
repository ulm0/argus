"use client";

import { memo } from "react";
import { AutopilotState, GearState } from "@/lib/sei-parser";
import type { SeiMetadata } from "@/lib/sei-parser";

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
  [AutopilotState.TACC]: "TACC",
};

function ArgusHUD({ sei, visible }: ArgusHUDProps) {
  if (!visible) return null;

  const speed = sei
    ? Math.round(Math.abs(sei.vehicleSpeedMps) * 2.23694)
    : 0;
  const gear = sei ? GEAR_LABELS[sei.gearState] ?? "P" : "P";
  const wheelAngle = sei ? sei.steeringWheelAngle : 0;
  const leftBlinker = sei?.blinkerOnLeft ?? false;
  const rightBlinker = sei?.blinkerOnRight ?? false;
  const brakeOn = sei?.brakeApplied ?? false;
  const throttle = sei
    ? Math.min(100, Math.max(0, sei.acceleratorPedalPosition <= 1.2
        ? sei.acceleratorPedalPosition * 100
        : sei.acceleratorPedalPosition))
    : 0;
  const apState = sei?.autopilotState ?? AutopilotState.NONE;
  const apLabel = AP_LABELS[apState] ?? "";

  return (
    <div
      className="pointer-events-none absolute left-1/2 top-3 z-20 flex -translate-x-1/2 flex-col items-center gap-1.5"
      style={{ transform: "translateX(-50%) translateZ(3px)" }}
    >
      <div
        className="border border-white/[0.14] shadow-[0_10px_30px_rgba(0,0,0,0.25)]"
        style={{
          padding: "6px 10px 8px",
          borderRadius: "18px",
          backdropFilter: "blur(14px)",
          background: "rgba(10, 10, 12, 0.38)",
        }}
      >
        {/* Grid: gear . left speed right . wheel / brake . autopilot autopilot autopilot . throttle */}
        <div
          className="items-center"
          style={{
            display: "grid",
            gridTemplateColumns: "auto 1fr 32px 3ch 32px 1fr auto",
            gridTemplateRows: "auto auto",
            gridTemplateAreas:
              '"gear . left speed right . wheel" "brake . autopilot autopilot autopilot . throttle"',
            columnGap: "8px",
            rowGap: "8px",
          }}
        >
          {/* Gear */}
          <div
            style={{ gridArea: "gear" }}
            className="rounded-md bg-white/10 px-3 py-1.5 text-xs font-extrabold text-white/95"
          >
            {gear}
          </div>

          {/* Left blinker */}
          <Blinker side="left" active={leftBlinker} />

          {/* Speed */}
          <div
            style={{ gridArea: "speed", transform: "translateY(16px)" }}
            className="flex flex-col items-center leading-none"
          >
            <span
              className="text-[28px] font-extrabold text-white/95"
              style={{ fontVariantNumeric: "tabular-nums" }}
            >
              {speed}
            </span>
            <span className="mt-1 text-xs text-white/80">mph</span>
          </div>

          {/* Right blinker */}
          <Blinker side="right" active={rightBlinker} />

          {/* Steering wheel */}
          <div
            style={{ gridArea: "wheel", transform: "translateY(16px)" }}
            className="flex h-8 w-8 items-center justify-center rounded-full border border-white/[0.18] bg-black/30"
          >
            <svg
              className="h-[26px] w-[26px] text-white/90"
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

          {/* Brake pedal */}
          <PedalIndicator
            gridArea="brake"
            fill={brakeOn ? 100 : 0}
            color="rgba(255, 90, 90, 0.75)"
            icon="brake"
          />

          {/* Autopilot */}
          <div
            style={{
              gridArea: "autopilot",
              maxHeight: apLabel ? "16px" : "0",
              opacity: apLabel ? 1 : 0,
              transition: "opacity 0.25s ease, max-height 0.25s ease",
            }}
            className="overflow-hidden text-center text-[11px] font-semibold tracking-wide text-[#4af]"
          >
            {apLabel}
          </div>

          {/* Throttle pedal */}
          <PedalIndicator
            gridArea="throttle"
            fill={throttle}
            color="rgba(120, 255, 120, 0.7)"
            icon="throttle"
          />
        </div>
      </div>
    </div>
  );
}

export default memo(ArgusHUD);

// ── Sub-components ──

function Blinker({ side, active }: { side: "left" | "right"; active: boolean }) {
  return (
    <div
      style={{
        gridArea: side,
        transform: "translateY(16px)",
      }}
      className={`
        grid h-6 w-8 place-items-center rounded-[10px] border text-sm
        transition-all
        ${active
          ? "border-[rgba(120,255,120,0.45)] bg-[rgba(120,255,120,0.18)] opacity-100 animate-pulse"
          : "border-white/[0.15] opacity-40"
        }
      `}
    >
      <svg
        className="h-4 w-4 text-white/90"
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

function PedalIndicator({
  gridArea,
  fill,
  color,
  icon,
}: {
  gridArea: string;
  fill: number;
  color: string;
  icon: "brake" | "throttle";
}) {
  const clampedFill = Math.min(100, Math.max(0, fill));

  return (
    <div
      style={{ gridArea, "--pedal-color": color } as React.CSSProperties}
      className="relative flex h-8 w-8 items-center justify-center rounded-full"
    >
      {/* Background ring */}
      <div className="absolute inset-1 rounded-full bg-black/40" />

      {/* Fill */}
      <div className="absolute inset-1 overflow-hidden rounded-full">
        <div
          className="absolute inset-x-0 bottom-0"
          style={{
            height: `${clampedFill}%`,
            background: color,
            transition: "height 0.1s ease-out",
          }}
        />
      </div>

      {/* Icon */}
      <svg
        className="relative z-10 h-6 w-6"
        viewBox="0 0 24 24"
        fill="none"
        stroke="rgba(136, 136, 136, 0.9)"
        strokeWidth={1.5}
      >
        {icon === "brake" ? (
          <>
            <circle cx="12" cy="12" r="8" />
            <line x1="8" y1="12" x2="16" y2="12" />
          </>
        ) : (
          <>
            <circle cx="12" cy="12" r="8" />
            <line x1="12" y1="8" x2="12" y2="16" />
            <line x1="8" y1="12" x2="16" y2="12" />
          </>
        )}
      </svg>
    </div>
  );
}
