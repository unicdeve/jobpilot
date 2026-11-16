import { expect, test } from 'bun:test';
import {
  defaultNotificationSettings,
  postable,
  scoutRunNotification,
  validateNotificationSettings,
  type NotificationKind,
  type RunTrigger,
  type ScoutRunOutcome,
  type ScoutVerdict,
} from './notification.ts';

// The cases are the Go test's (internal/core/domain/notification_test.go).
const created = (score: number): ScoutVerdict => ({
  eligible: true,
  hasScore: true,
  score,
  considered: true,
});
const rejected = (score: number): ScoutVerdict => ({
  eligible: false,
  hasScore: true,
  score,
  considered: true,
  reason: 'below the floor',
});
const outcome = (
  trigger: RunTrigger,
  failed: boolean,
  ...verdicts: ScoutVerdict[]
): ScoutRunOutcome => ({
  scoutId: 'scout-1',
  scoutName: 'Morning Go roles',
  runId: 'run-1',
  trigger,
  report: { verdicts },
  failed,
});

const cases: [string, ScoutRunOutcome, NotificationKind, string, string][] = [
  [
    'a scheduled run that created applications',
    outcome('schedule', false, created(72), created(75), rejected(90)),
    'scout_found',
    'Morning Go roles found 2 jobs for you',
    'They’re ready for review.',
  ],
  [
    'some strong matches say both, once',
    outcome('schedule', false, created(72), created(88), created(91)),
    'scout_found',
    'Morning Go roles found 3 jobs for you',
    '2 of them are strong matches. They’re ready for review.',
  ],
  [
    'one strong match among several',
    outcome('schedule', false, created(72), created(88)),
    'scout_found',
    'Morning Go roles found 2 jobs for you',
    '1 of them is a strong match. They’re ready for review.',
  ],
  [
    'a manual run tells only about a strong match',
    outcome('manual', false, created(85)),
    'scout_found',
    'Morning Go roles found 1 job for you',
    'It’s a strong match, ready for review.',
  ],
  [
    'every one strong',
    outcome('manual', false, created(85), created(80)),
    'scout_found',
    'Morning Go roles found 2 jobs for you',
    'All of them are strong matches, ready for review.',
  ],
  [
    'a scheduled run that failed',
    outcome('schedule', true),
    'scout_failed',
    'Morning Go roles couldn’t run',
    'Open JobPilot to see why.',
  ],
  [
    'failed after creating something',
    outcome('schedule', true, created(70)),
    'scout_found',
    'Morning Go roles found 1 job for you',
    'It’s ready for review.',
  ],
];
test.each(cases)('%s', (_n, run, kind, title, body) => {
  expect(scoutRunNotification(run, 80)).toEqual({
    kind,
    title,
    body,
    scoutId: 'scout-1',
    runId: 'run-1',
  });
});

test('says nothing', () => {
  const none: ScoutRunOutcome[] = [
    outcome('schedule', false, rejected(95)),
    outcome('manual', false, created(79)),
    outcome('manual', true),
    outcome('manual', false, rejected(99)),
    { ...outcome('manual', false), report: null },
  ];
  for (const run of none) expect(scoutRunNotification(run, 80)).toBeNull();
});

test('the threshold is the users', () => {
  const run = outcome('manual', false, created(70));
  expect(scoutRunNotification(run, 80)).toBeNull();
  expect(scoutRunNotification(run, 70)).not.toBeNull();
});

test('says nothing about the job', () => {
  const run = outcome('schedule', false, {
    jobId: 'job-1',
    jobTitle: 'Senior Go Engineer at Acme',
    eligible: true,
    hasScore: true,
    score: 93,
    reason: "match score 93 clears the scout's floor of 70",
  });
  const got = scoutRunNotification(run, 80)!;
  for (const leak of ['Senior', 'Acme', '93', 'floor'])
    expect(got.title + got.body).not.toContain(leak);
});

test('postable', () => {
  const now = Date.UTC(2026, 9, 3, 8);
  expect(postable({ createdAt: now - 10 * 60000, postedAt: null }, now)).toBe(true);
  expect(postable({ createdAt: now - 2 * 3600000, postedAt: null }, now)).toBe(false);
  expect(postable({ createdAt: now, postedAt: now }, now)).toBe(false);
});

test('settings', () => {
  expect(defaultNotificationSettings()).toEqual({ enabled: true, strongMatchScore: 80 });
  expect(validateNotificationSettings(defaultNotificationSettings())).toBeNull();
  expect(validateNotificationSettings({ enabled: false, strongMatchScore: 0 })).not.toBeNull();
  expect(validateNotificationSettings({ enabled: false, strongMatchScore: 101 })).not.toBeNull();
});
