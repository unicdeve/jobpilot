// A port of the Go core's Scout schedule rule (internal/core/domain/scout_schedule.go, ADR-0027 and ADR-0037).
// It has no Exact, network or storage dependency and reads no clock: `now` and `since` are epoch milliseconds the caller passes.
// Not ported: ScoutAlarm and its fingerprint (they describe operating-system alarms, which an on-device core does not have)
// and the alarm, activity and wake status types.
//
// Time zone: a daily time is wall-clock time in the computer's zone, as in Go.
// The Go code carries the zone on each time.Time; here it is a Zone, which defaults to the JS engine's local zone.
// Because every value is an epoch instant, "put since in now's zone" (Go's since.In(now.Location())) needs no step.

export type ScheduleKind = 'off' | 'every' | 'daily';

export interface ScoutSchedule {
  kind: ScheduleKind | string;
  // The interval for 'every', 1 to 24.
  everyHours: number;
  // The time of day for 'daily', in minutes after the computer's local midnight.
  dailyAt: number;
}

// Where wall-clock days are read.
export interface Zone {
  // The calendar day (month 1 to 12) an instant falls on.
  day(ms: number): { year: number; month: number; day: number };
  // The instant of a wall-clock time on a calendar day.
  at(year: number, month: number, day: number, hour: number, minute: number): number;
}

export const localZone: Zone = {
  day: (ms) => {
    const d = new Date(ms);
    return { year: d.getFullYear(), month: d.getMonth() + 1, day: d.getDate() };
  },
  at: (year, month, day, hour, minute) => {
    const d = new Date(2000, month - 1, day, hour, minute, 0, 0);
    d.setFullYear(year, month - 1, day); // new Date(y < 100, ...) would read the year as 19xx
    return d.getTime();
  },
};

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;

// How early an alarm may fire and still count as the scheduled time (ADR-0027 section 2).
export const scheduleGrace = 5 * MINUTE;

export const isScheduled = (s: ScoutSchedule): boolean => s.kind === 'every' || s.kind === 'daily';

// An empty kind reads as off, and fields the kind does not use are cleared.
export const normalizedSchedule = (s: ScoutSchedule): ScoutSchedule => {
  switch (s.kind) {
    case 'every':
      return { kind: 'every', everyHours: s.everyHours, dailyAt: 0 };
    case 'daily':
      return { kind: 'daily', everyHours: 0, dailyAt: s.dailyAt };
    case '':
    case 'off':
      return { kind: 'off', everyHours: 0, dailyAt: 0 };
  }
  return s;
};

// Says what is wrong, or returns null when the schedule can be kept.
export const validateSchedule = (s: ScoutSchedule): string | null => {
  switch (s.kind) {
    case '':
    case 'off':
      return null;
    case 'every':
      return s.everyHours < 1 || s.everyHours > 24
        ? `a scout can run every 1 to 24 hours, got ${s.everyHours}`
        : null;
    case 'daily':
      return s.dailyAt < 0 || s.dailyAt >= 24 * 60
        ? `a daily time must be between 00:00 and 23:59, got minute ${s.dailyAt}`
        : null;
  }
  return `unknown schedule "${s.kind}"`;
};

// minutes after midnight on the date of `day`, built from the date so a 23 or 25 hour day still gives the wall-clock time asked for.
const dailyMoment = (
  zone: Zone,
  day: { year: number; month: number; day: number },
  minutes: number,
): number => zone.at(day.year, day.month, day.day, Math.floor(minutes / 60), minutes % 60);

// Go's AddDate(0, 0, 1) on a calendar day.
const nextDay = (d: {
  year: number;
  month: number;
  day: number;
}): { year: number; month: number; day: number } => {
  const u = new Date(0);
  u.setUTCFullYear(d.year, d.month - 1, d.day + 1);
  return { year: u.getUTCFullYear(), month: u.getUTCMonth() + 1, day: u.getUTCDate() };
};

// The moment the Scout becomes due, given since: the later of its last run's start and when its schedule was set.
// A null since (never run, no record) is due at once (0).
// Returns null for an unscheduled kind.
export const dueAt = (
  s: ScoutSchedule,
  since: number | null,
  zone: Zone = localZone,
): number | null => {
  if (since === null) return 0;
  switch (s.kind) {
    case 'every':
      return since + s.everyHours * HOUR - scheduleGrace;
    case 'daily': {
      // The first daily time after since, past the grace an alarm is allowed.
      const after = since + scheduleGrace;
      let at = dailyMoment(zone, zone.day(after), s.dailyAt);
      if (!(at > after)) at = dailyMoment(zone, nextDay(zone.day(after)), s.dailyAt);
      return at - scheduleGrace;
    }
  }
  return null;
};

// Whether the Scout should run now.
export const isDue = (
  s: ScoutSchedule,
  now: number,
  since: number | null,
  zone: Zone = localZone,
): boolean => {
  if (!isScheduled(s)) return false;
  const due = dueAt(s, since, zone);
  return due !== null && now >= due;
};

// When the hourly alarm will next find the Scout due, for showing the user.
// A Scout already due runs at its next alarm.
// Returns null for an unscheduled kind.
export const nextRun = (
  s: ScoutSchedule,
  now: number,
  since: number | null,
  zone: Zone = localZone,
): number | null => {
  let due = dueAt(s, since, zone);
  if (due === null) return null;
  if (due < now) due = now;
  switch (s.kind) {
    case 'every': {
      // Go's Truncate(time.Hour) is on the absolute hour, which is the epoch's.
      let hour = Math.floor(due / HOUR) * HOUR;
      if (hour < due) hour += HOUR;
      return hour;
    }
    case 'daily': {
      let at = dailyMoment(zone, zone.day(due), s.dailyAt);
      if (at < due) at = dailyMoment(zone, nextDay(zone.day(due)), s.dailyAt);
      return at;
    }
  }
  return null;
};
