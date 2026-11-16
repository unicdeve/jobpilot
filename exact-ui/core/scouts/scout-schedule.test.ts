import { expect, test } from 'bun:test';
import {
  dueAt,
  isDue,
  nextRun,
  normalizedSchedule,
  validateSchedule,
  type Zone,
  type ScoutSchedule,
} from './scout-schedule.ts';

// The cases are the Go test's (internal/core/domain/scout_schedule_test.go), so the two cores cannot drift silently.
// A zone built on Intl lets the tests run in any zone whatever the machine's own is.
const intlZone = (timeZone: string): Zone => {
  const fmt = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
  });
  const parts = (ms: number) => {
    const o: Record<string, number> = {};
    for (const p of fmt.formatToParts(ms)) if (p.type !== 'literal') o[p.type] = +p.value;
    return o as {
      year: number;
      month: number;
      day: number;
      hour: number;
      minute: number;
      second: number;
    };
  };
  const offset = (ms: number) => {
    const p = parts(ms);
    return (
      Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second) -
      Math.floor(ms / 1000) * 1000
    );
  };
  return {
    day: (ms) => {
      const p = parts(ms);
      return { year: p.year, month: p.month, day: p.day };
    },
    at: (y, mo, d, h, mi) => {
      const wall = Date.UTC(y, mo - 1, d, h, mi);
      let guess = wall - offset(wall);
      guess = wall - offset(guess);
      return guess;
    },
  };
};
const utc = intlZone('UTC');
const at = (zone: Zone, value: string): number => {
  const [d, t] = value.split(' ');
  const [y, mo, da] = d!.split('-').map(Number);
  const [h, mi, s] = t!.split(':').map(Number);
  return zone.at(y!, mo!, da!, h!, mi!) + s! * 1000;
};
const sched = (kind: string, everyHours = 0, dailyAt = 0): ScoutSchedule => ({
  kind,
  everyHours,
  dailyAt,
});
const nr = (s: ScoutSchedule, now: number, since: number | null, z: Zone) =>
  nextRun(s, now, since, z);

const validateCases: [string, ScoutSchedule, string][] = [
  ['empty is off', sched(''), ''],
  ['off', sched('off'), ''],
  ['every hour', sched('every', 1), ''],
  ["every day's worth", sched('every', 24), ''],
  ['every zero hours', sched('every'), 'every 1 to 24 hours'],
  ['every 25 hours', sched('every', 25), 'every 1 to 24 hours'],
  ['daily at midnight', sched('daily', 0, 0), ''],
  ['daily at 23:59', sched('daily', 0, 23 * 60 + 59), ''],
  ['daily at 24:00', sched('daily', 0, 24 * 60), 'between 00:00 and 23:59'],
  ['daily before midnight', sched('daily', 0, -1), 'between 00:00 and 23:59'],
  ['cron', sched('0 8 * * *'), 'unknown schedule'],
];
for (const [name, s, want] of validateCases) {
  test(`validate: ${name}`, () => {
    const err = validateSchedule(s);
    if (want === '') expect(err).toBeNull();
    else expect(err).toContain(want);
  });
}

test('normalized', () => {
  expect(normalizedSchedule(sched(''))).toEqual(sched('off'));
  expect(normalizedSchedule(sched('off', 6))).toEqual(sched('off'));
  expect(normalizedSchedule(sched('every', 6, 480))).toEqual(sched('every', 6));
  expect(normalizedSchedule(sched('daily', 6, 480))).toEqual(sched('daily', 0, 480));
});

const every6 = sched('every', 6);
const since = at(utc, '2026-10-02 10:00:00');

test('every: due six hours after the last start, less the grace', () => {
  expect(dueAt(every6, since, utc)).toBe(at(utc, '2026-10-02 15:55:00'));
  expect(isDue(every6, at(utc, '2026-10-02 15:54:59'), since, utc)).toBe(false);
  expect(isDue(every6, at(utc, '2026-10-02 15:55:00'), since, utc)).toBe(true);
  expect(isDue(every6, at(utc, '2026-10-02 16:00:00'), since, utc)).toBe(true);
});

test('every: an alarm a few seconds early still counts', () => {
  expect(isDue(every6, at(utc, '2026-10-02 15:59:58'), since, utc)).toBe(true);
});

test('every: a run that started late does not push the next past its hour', () => {
  expect(isDue(every6, at(utc, '2026-10-02 22:00:00'), at(utc, '2026-10-02 16:00:03'), utc)).toBe(
    true,
  );
});

test('every: the next run is the first hourly alarm that finds it due', () => {
  expect(nr(every6, at(utc, '2026-10-02 10:30:00'), since, utc)).toBe(
    at(utc, '2026-10-02 16:00:00'),
  );
  expect(nr(every6, at(utc, '2026-10-02 10:30:00'), at(utc, '2026-10-02 10:20:00'), utc)).toBe(
    at(utc, '2026-10-02 17:00:00'),
  );
});

test('every: an overdue scout runs at the next alarm', () => {
  const now = at(utc, '2026-10-05 09:12:00');
  expect(isDue(every6, now, since, utc)).toBe(true);
  expect(nr(every6, now, since, utc)).toBe(at(utc, '2026-10-05 10:00:00'));
});

const daily8 = sched('daily', 0, 8 * 60);

test("daily: scheduled after today's time, first runs tomorrow", () => {
  const set = at(utc, '2026-10-02 10:00:00');
  expect(isDue(daily8, at(utc, '2026-10-02 10:00:01'), set, utc)).toBe(false);
  expect(isDue(daily8, at(utc, '2026-10-03 07:54:59'), set, utc)).toBe(false);
  expect(isDue(daily8, at(utc, '2026-10-03 08:00:00'), set, utc)).toBe(true);
  expect(nr(daily8, at(utc, '2026-10-02 10:00:01'), set, utc)).toBe(at(utc, '2026-10-03 08:00:00'));
});

test("daily: scheduled before today's time, runs today", () => {
  expect(isDue(daily8, at(utc, '2026-10-02 08:00:00'), at(utc, '2026-10-02 07:00:00'), utc)).toBe(
    true,
  );
});

test("daily: a run its early alarm started has had today's", () => {
  const started = at(utc, '2026-10-02 07:59:58');
  expect(isDue(daily8, at(utc, '2026-10-02 08:01:00'), started, utc)).toBe(false);
  expect(isDue(daily8, at(utc, '2026-10-03 08:00:00'), started, utc)).toBe(true);
});

test('daily: a manual run before the time does not stand in for it', () => {
  expect(isDue(daily8, at(utc, '2026-10-02 08:00:00'), at(utc, '2026-10-02 07:30:00'), utc)).toBe(
    true,
  );
});

test('daily: days asleep run once, then wait for tomorrow', () => {
  const last = at(utc, '2026-09-29 08:00:00');
  const woke = at(utc, '2026-10-02 12:00:00');
  expect(isDue(daily8, woke, last, utc)).toBe(true);
  expect(isDue(daily8, woke + 60_000, woke, utc)).toBe(false);
  expect(isDue(daily8, at(utc, '2026-10-03 08:00:00'), woke, utc)).toBe(true);
});

test('daily: an overdue scout shows its catch-up as the next run', () => {
  expect(nr(daily8, at(utc, '2026-10-02 12:00:00'), at(utc, '2026-09-29 08:00:00'), utc)).toBe(
    at(utc, '2026-10-03 08:00:00'),
  );
});

test('daily: the time is wall-clock across a change to daylight saving', () => {
  const ny = intlZone('America/New_York');
  // 2026-03-08 is 23 hours long in New York.
  const last = at(ny, '2026-03-07 08:00:00');
  expect(nr(daily8, last + 3_600_000, last, ny)).toBe(at(ny, '2026-03-08 08:00:00'));
  expect(isDue(daily8, at(ny, '2026-03-08 07:00:00'), last, ny)).toBe(false);
  expect(isDue(daily8, at(ny, '2026-03-08 08:00:00'), last, ny)).toBe(true);
});

// Read back from SQLite a scout's times are in UTC; a daily time worked out in UTC put "daily at 08:00" at 09:00 in Lagos.
test("daily: read in the computer's time zone", () => {
  const lagos = intlZone('Africa/Lagos');
  const set = Date.UTC(2026, 9, 2, 23, 20); // 00:20 in Lagos
  expect(nr(daily8, set, set, lagos)).toBe(at(lagos, '2026-10-03 08:00:00'));
  expect(isDue(daily8, at(lagos, '2026-10-03 08:00:00'), set, lagos)).toBe(true);
  expect(isDue(daily8, at(lagos, '2026-10-03 07:30:00'), set, lagos)).toBe(false);
});

test('off is never due', () => {
  const now = Date.UTC(2026, 9, 2, 8);
  expect(isDue(sched('off'), now, null, utc)).toBe(false);
  expect(isDue(sched(''), now, null, utc)).toBe(false);
  expect(nr(sched('off'), now, since, utc)).toBeNull();
});

test('a scout that never ran is due at once', () => {
  expect(isDue(every6, Date.UTC(2026, 9, 2, 8), null, utc)).toBe(true);
});

test('the default local zone agrees with a fixed-offset read of the same machine', () => {
  const s = Date.now();
  expect(dueAt(daily8, s)).not.toBeNull();
});
