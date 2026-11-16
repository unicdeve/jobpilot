// Tracking a job as an application on the device (ADR-0037), ported from ApplicationService.CreateApplication,
// UpdateState and SetApplyURL. The module is pure: the data source reads and writes SQLite, and passes the
// clock in (a data source cannot read it). It never writes application content for the candidate (ADR-0034).
import { canTransition, nextStates, validApplyUrl, type ApplicationState } from './application.ts';

export type TrackedJob = { id: string; title: string; company: string; url: string };
export type Tracked = {
  id: string;
  jobId: string;
  title: string;
  company: string;
  url: string;
  applyUrl: string;
  state: ApplicationState;
  updatedAt: number;
};
export type Transition = {
  id: string;
  applicationId: string;
  from: string;
  to: string;
  reason: string;
  at: number;
};

// One application per job, so the job names it and a second track finds the first.
export const applicationId = (jobId: string): string => `local:${jobId}`;
export const isLocal = (id: string): boolean => id.startsWith('local:');

// A new application starts DISCOVERED, as CreateApplication does. Its link starts as the job's own page, which the
// user can replace with the page they apply on.
export function newApplication(job: TrackedJob, now: number): Tracked | undefined {
  if (job.id.trim() === '' || job.title.trim() === '') return undefined;
  return {
    id: applicationId(job.id),
    jobId: job.id,
    title: job.title.trim(),
    company: job.company.trim(),
    url: job.url,
    applyUrl: validApplyUrl(job.url) ? job.url.trim() : '',
    state: 'DISCOVERED',
    updatedAt: now,
  };
}

export type Moved =
  { ok: true; application: Tracked; step: Transition } | { ok: false; error: string };

// Only the states the rule table allows from the current one (application.ts). `seq` makes the step's id.
export function move(app: Tracked, to: string, reason: string, now: number, seq: number): Moved {
  if (!canTransition(app.state, to))
    return { ok: false, error: `An application cannot go from ${app.state} to ${to}.` };
  return {
    ok: true,
    application: { ...app, state: to as ApplicationState, updatedAt: now },
    step: {
      id: `${app.id}:${seq}`,
      applicationId: app.id,
      from: app.state,
      to,
      reason: reason.trim(),
      at: now,
    },
  };
}

export type Linked = { ok: true; application: Tracked } | { ok: false; error: string };

// The link is only ever opened in the user's browser (ADR-0034); an empty one clears it.
export function setApplyLink(app: Tracked, raw: string, now: number): Linked {
  if (!validApplyUrl(raw))
    return { ok: false, error: 'The link must be a full http or https address.' };
  return { ok: true, application: { ...app, applyUrl: raw.trim(), updatedAt: now } };
}

export const choices = (state: string): ApplicationState[] => nextStates(state);

export const day = (ms: number): string => new Date(ms).toISOString().slice(0, 10);
