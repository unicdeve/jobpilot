// A port of the Go core's Scout rules (internal/core/domain/scout.go), ADR-0037. The schedule is in scoutSchedule.ts.
// Pure: no Exact, network, storage or clock dependency; the caller passes `now`.
// Not ported: the run loop in services/scout.go (repositories, scoring, notifier). Work-from and fit-hours stay unported, as in criteria.ts.
import { matches, type Job, type SearchCriteria } from './criteria.ts';
import { validateSchedule, type ScoutSchedule } from './scout-schedule.ts';

export type ScoutSource = 'local' | 'sources';
export const scoutSources: ScoutSource[] = ['local', 'sources'];
export const isValidSource = (s: string): s is ScoutSource => (scoutSources as string[]).includes(s);


export interface Scout {
  name: string;
  savedSearchId: string;
  minMatchScore: number;
  maxApplicationsPerRun: number;
  source: string;
  isEnabled: boolean;
  schedule: ScoutSchedule;
}

// What a run reads of a job: the search's Job plus the user's "not interested" mark.
export type ScoutJob = Job & { id?: string; notInterestedAt?: string | null };

export interface EligibilityInput {
  job: ScoutJob | null;
  criteria: SearchCriteria;
  // A job that could not be scored is neither zero nor clearing the floor.
  score: number;
  hasScore: boolean;
  alreadyApplied: boolean;
  createdSoFar: number;
}

export const maxApplicationsPerRunCeiling = 50;

// Returns an error message, or '' when the scout is usable.
export function validateScout(s: Scout): string {
  if (s.name.trim() === '') return 'a scout needs a name';
  if (s.savedSearchId.trim() === '') return 'a scout needs a saved search to run';
  if (!isValidSource(s.source)) return `unknown scout source "${s.source}"`;
  if (s.minMatchScore < 0 || s.minMatchScore > 100) return `the minimum match score must be between 0 and 100, got ${s.minMatchScore}`;
  if (s.maxApplicationsPerRun < 1 || s.maxApplicationsPerRun > maxApplicationsPerRunCeiling) {
    return `a run must be allowed to create between 1 and ${maxApplicationsPerRunCeiling} applications, got ${s.maxApplicationsPerRun}`;
  }
  return validateSchedule(s.schedule) ?? '';
}

// Everything that needs no match score, so a caller can skip the model call.
export function preScreen(s: Scout, input: EligibilityInput): [boolean, string] {
  const job = input.job;
  if (job === null || job === undefined) return [false, 'the job could not be read'];
  if (job.notInterestedAt) return [false, 'you marked this job not interested'];
  if (!matches(input.criteria, job)) return [false, 'does not match the saved search'];
  if (input.alreadyApplied) return [false, 'an application for this job already exists'];
  if (input.createdSoFar >= s.maxApplicationsPerRun) {
    return [false, `this run already created its limit of ${s.maxApplicationsPerRun} applications`];
  }
  return [true, ''];
}

export function eligible(s: Scout, input: EligibilityInput): [boolean, string] {
  const [ok, reason] = preScreen(s, input);
  if (!ok) return [false, reason];
  if (!input.hasScore) return [false, 'could not be scored against the candidate'];
  if (input.score < s.minMatchScore) return [false, `match score ${input.score} is below the scout's floor of ${s.minMatchScore}`];
  return [true, `match score ${input.score} clears the scout's floor of ${s.minMatchScore}`];
}
