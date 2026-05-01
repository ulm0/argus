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
};

// Prefixes we strip before title-casing unknown values, so a hypothetical
// `user_interaction_door_handle` renders as "Door handle" rather than
// "User interaction door handle".
const STRIP_PREFIXES = [
  "user_interaction_",
  "user_command_",
  "sentry_aware_",
  "sentry_",
];

export function formatEventReason(raw: string | null | undefined): string {
  if (!raw) return "";
  const key = raw.trim().toLowerCase();
  if (KNOWN_REASONS[key]) return KNOWN_REASONS[key];

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
