// Wording for a scout's schedule, shared by the scouts screen and anything
// that mentions when a scout runs.

import type { Scout, ScoutAlarmStatus, ScoutSchedule } from "./types";

/// timeOfDay reads minutes after midnight as the user's clock shows a time.
export function timeOfDay(minutes: number): string {
  const at = new Date(2000, 0, 1, Math.floor(minutes / 60), minutes % 60);
  return at.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}

/// scheduleLabel says when a scout runs, to follow "Runs".
export function scheduleLabel(schedule: ScoutSchedule | undefined): string {
  switch (schedule?.kind) {
    case "every":
      return schedule.every_hours === 1 ? "every hour" : `every ${schedule.every_hours} hours`;
    case "daily":
      return `daily at ${timeOfDay(schedule.daily_at ?? 0)}`;
    default:
      return "only when you run it";
  }
}

/// nextRunLabel says when a scheduled run is next: "today at 8:00 AM",
/// "tomorrow at 8:00 AM", or a date beyond that.
export function nextRunLabel(iso: string): string {
  const at = new Date(iso);
  const time = at.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  const days = Math.round((startOfDay(at) - startOfDay(new Date())) / 86_400_000);
  if (days <= 0) return `today at ${time}`;
  if (days === 1) return `tomorrow at ${time}`;
  return at.toLocaleString(undefined, { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
}

function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
}

/// alarmProblem says why a scheduled scout will not run on its own, or
/// nothing when its alarm is in place.
export function alarmProblem(alarm: ScoutAlarmStatus | undefined): string | undefined {
  switch (alarm?.state) {
    case "missing":
      return `Not scheduled with your computer: ${alarm.reason ?? "the reason was not given"}. It runs only when you run it until this is fixed.`;
    case "switched_off":
      return isMac()
        ? "Switched off in System Settings → General → Login Items, so it won’t run on its own. Turn JobPilot back on there to resume."
        : "Switched off outside JobPilot, so it won’t run on its own.";
    case "unavailable":
      return "This computer has no scheduler JobPilot can use, so it runs only when you run it.";
    default:
      return undefined;
  }
}

/// isScheduled reports a scout that runs on its own.
export function isScheduled(scout: Pick<Scout, "schedule" | "is_enabled">): boolean {
  return scout.is_enabled && (scout.schedule?.kind === "every" || scout.schedule?.kind === "daily");
}

/// isMac reports whether the desktop is running on macOS, where scheduling
/// shows the user a notice of its own.
export function isMac(): boolean {
  return /Mac/i.test(navigator.userAgent);
}
