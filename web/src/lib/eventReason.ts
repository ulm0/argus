// Tesla's event.json `reason` field uses snake_case identifiers (e.g.
// `user_interaction_honk`, `sentry_aware_object_detection`). Showing them
// raw in the UI is jarring; this util maps the known canonical values to
// human-friendly labels and prettifies anything unrecognised so future
// firmware changes don't render gibberish.

const KNOWN_REASONS: Record<string, string> = {
  sentry_aware_object_detection: "Sentry: object detected",
  sentry_aware_object_detected: "Sentry: object detected",
  user_interaction_honk: "Honk",
  user_interaction_dashcam_panel_save: "Manual save",
  user_interaction_dashcam_icon_tapped: "Manual save",
  user_interaction_dashcam_launcher_action_on: "Manual save",
  user_command_save_recording: "Manual save",
  user_command_dashcam_save: "Manual save",
  sentry_panic_mode: "Sentry: alarm triggered",
};

// Tesla appends the measured magnitude to accelerometer triggers
// (`sentry_aware_accel_0.490456`), so this one can never be a table lookup —
// and it is the "someone hit the car" event, the one that must not render
// as debug output.
const ACCEL_REASON = /^sentry_aware_accel/;

// Tesla's event.json `camera` field is the index of the triggering camera in
// its own clip ordering. Same ordering the player uses for the camera strip.
const CAMERA_BY_INDEX = [
  "front",
  "left_repeater",
  "right_repeater",
  "back",
  "left_pillar",
  "right_pillar",
] as const;

/** Maps event.json's numeric `camera` to a camera name; undefined if unknown. */
export function cameraFromIndex(
  raw: string | number | null | undefined,
): (typeof CAMERA_BY_INDEX)[number] | undefined {
  if (raw === null || raw === undefined || raw === "") return undefined;
  return CAMERA_BY_INDEX[Number(raw)];
}

// Prefixes we strip before title-casing unknown values, so a hypothetical
// `user_interaction_door_handle` renders as "Door handle" rather than
// "User interaction door handle".
const STRIP_PREFIXES = [
  "user_interaction_",
  "user_command_",
  "sentry_aware_",
  "sentry_",
];

/**
 * Coarse bucket for the reason filter chips. Deliberately a prefix test on the
 * same prefixes STRIP_PREFIXES knows about, not a lookup: Tesla adds `reason`
 * values with every firmware, and an unrecognised `sentry_*` must still land in
 * the Sentry bucket rather than disappear out of the filter entirely.
 */
export function eventReasonKind(
  raw: string | null | undefined,
): "sentry" | "user" | "other" {
  const key = (raw ?? "").trim().toLowerCase();
  if (key.startsWith("sentry_")) return "sentry";
  if (key.startsWith("user_")) return "user";
  return "other";
}

export function formatEventReason(raw: string | null | undefined): string {
  if (!raw) return "";
  const key = raw.trim().toLowerCase();
  if (KNOWN_REASONS[key]) return KNOWN_REASONS[key];
  if (ACCEL_REASON.test(key)) return "Sentry: vehicle disturbance";

  let stripped = key;
  for (const p of STRIP_PREFIXES) {
    if (stripped.startsWith(p)) {
      stripped = stripped.slice(p.length);
      break;
    }
  }
  const words = stripped.replace(/[_-]+/g, " ").trim();
  if (!words) return raw;
  return words.charAt(0).toUpperCase() + words.slice(1);
}
