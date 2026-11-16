// A port of the Go core's notification rules (internal/core/domain/notification.go), ADR-0037, ADR-0028.
// Text says counts and a scout's name and nothing else, because a notification shows on a lock screen (ADR-0028 section 3).
// Pure: no Exact, network or storage dependency; `now` is a parameter. Times are epoch milliseconds.

export type NotificationKind = 'scout_found' | 'scout_failed';
export type RunTrigger = 'manual' | 'schedule';

export interface Notification {
  id: string;
  kind: NotificationKind;
  title: string;
  body: string;
  scoutId?: string;
  runId?: string;
  createdAt: number;
  postedAt: number | null;
  seenAt: number | null;
}

// How long a notification may wait to be posted: one about this morning, shown at dinner, misleads (ADR-0028 section 4).
export const notificationPostWindowMs = 60 * 60 * 1000;

export const postable = (n: Pick<Notification, 'createdAt' | 'postedAt'>, now: number): boolean =>
  n.postedAt === null && now - n.createdAt <= notificationPostWindowMs;

export interface NotificationSettings {
  enabled: boolean;
  strongMatchScore: number;
}

export const defaultNotificationSettings = (): NotificationSettings => ({ enabled: true, strongMatchScore: 80 });

// Go returns an error; here, the message or null when the settings can be kept.
export const validateNotificationSettings = (s: NotificationSettings): string | null =>
  s.strongMatchScore < 1 || s.strongMatchScore > 100
    ? `a strong match must need a score between 1 and 100, got ${s.strongMatchScore}`
    : null;

export interface ScoutVerdict {
  jobId?: string;
  jobTitle?: string;
  eligible: boolean;
  reason?: string;
  score?: number;
  hasScore: boolean;
  considered?: boolean;
}

export interface ScoutRunOutcome {
  scoutId: string;
  scoutName: string;
  runId: string;
  trigger: RunTrigger;
  report: { verdicts: ScoutVerdict[] } | null;
  failed: boolean;
}

export interface ScoutRunNotice {
  kind: NotificationKind;
  title: string;
  body: string;
  scoutId: string;
  runId: string;
}

const count = (n: number, one: string, many: string): string => (n === 1 ? `1 ${one}` : `${n} ${many}`);

const readyForReview = (created: number): string => (created === 1 ? 'It’s ready for review.' : 'They’re ready for review.');

const foundBody = (created: number, strong: number): string => {
  if (strong === 0) return readyForReview(created);
  if (strong === created && created === 1) return 'It’s a strong match, ready for review.';
  if (strong === created) return 'All of them are strong matches, ready for review.';
  if (strong === 1) return `1 of them is a strong match. ${readyForReview(created)}`;
  return `${strong} of them are strong matches. ${readyForReview(created)}`;
};

// Whether a finished scout run tells the user, and what it says; null when it says nothing.
// A scheduled run tells when it created applications or failed; any run tells when it created one for a strong match.
export function scoutRunNotification(run: ScoutRunOutcome, strongScore: number): ScoutRunNotice | null {
  let created = 0;
  let strong = 0;
  for (const v of run.report?.verdicts ?? []) {
    if (!v.eligible) continue;
    created++;
    if (v.hasScore && (v.score ?? 0) >= strongScore) strong++;
  }
  const scheduled = run.trigger === 'schedule';
  const base = { scoutId: run.scoutId, runId: run.runId };
  if (created > 0 && (scheduled || strong > 0)) {
    return { ...base, kind: 'scout_found', title: `${run.scoutName} found ${count(created, 'job', 'jobs')} for you`, body: foundBody(created, strong) };
  }
  if (scheduled && run.failed) {
    return { ...base, kind: 'scout_failed', title: `${run.scoutName} couldn’t run`, body: 'Open JobPilot to see why.' };
  }
  return null;
}
