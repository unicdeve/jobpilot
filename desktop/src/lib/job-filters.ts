// The filters on the Jobs screen, as they are kept in the URL: a link can
// open a filtered list, and selecting a job keeps the list as it was.
//
// "All jobs" is paged by the daemon, so its filters are sent with each
// request. "Analyzed" holds every match already, so its filters — the score
// and when the job was analyzed among them — are applied here.

import type { Job, JobMatch } from "./types";
import { REMOTE_LABELS, SOURCE_LABELS } from "./format";

/// TIME_RANGES are the windows a date filter offers, keyed by how they
/// appear in the URL.
export const TIME_RANGES = {
  "1d": { label: "Past 24 hours", days: 1 },
  "3d": { label: "Past 3 days", days: 3 },
  "7d": { label: "Past week", days: 7 },
  "30d": { label: "Past month", days: 30 },
} as const;

export type TimeRange = keyof typeof TIME_RANGES;

/// SCORE_FLOORS are the minimum match scores the Analyzed view offers.
export const SCORE_FLOORS = {
  "80": "80 and above",
  "60": "60 and above",
  "40": "40 and above",
} as const;

export type ScoreFloor = keyof typeof SCORE_FLOORS;

/// WHERE_CHOICES narrow the list by where the candidate can take a job from,
/// as a saved search's "I can work from" does: jobs open in their country
/// or anywhere, and those that do not say; with "relocate", also jobs
/// elsewhere that offer or mention relocation or visa sponsorship. Both
/// need the profile's country, which the labels name.
export const WHERE_CHOICES = {
  open: "open",
  relocate: "relocate",
} as const;

export type WhereChoice = keyof typeof WHERE_CHOICES;

export type RemoteType = keyof typeof REMOTE_LABELS;

/// JobFilters is every filter the screen knows. Each view reads only the
/// ones that apply to it; an empty string means "not filtered".
export interface JobFilters {
  source: string;
  remote: RemoteType | "";
  /// When the job was posted — the "All jobs" view.
  posted: TimeRange | "";
  /// When the job was analyzed — the "Analyzed" view.
  analyzed: TimeRange | "";
  score: ScoreFloor | "";
  /// Where the candidate can take a job from — the "All jobs" view.
  where: WhereChoice | "";
}

/// VIEW_FILTERS are the filters each view applies, and so the ones its
/// filter menu shows and counts.
export const VIEW_FILTERS = {
  all: ["where", "posted", "source", "remote"],
  analyzed: ["score", "analyzed", "source", "remote"],
  not_interested: ["where", "posted", "source", "remote"],
} as const satisfies Record<string, readonly (keyof JobFilters)[]>;

export type JobsView = keyof typeof VIEW_FILTERS;

const pick = <T extends string>(options: Record<T, unknown>, value: string | null): T | "" =>
  value !== null && Object.prototype.hasOwnProperty.call(options, value) ? (value as T) : "";

/// readJobFilters reads the filters from the URL. A value the screen does not
/// offer is dropped rather than sent, so a stale link opens unfiltered
/// instead of failing.
export function readJobFilters(params: URLSearchParams): JobFilters {
  return {
    source: pick(SOURCE_LABELS, params.get("source")),
    remote: pick(REMOTE_LABELS, params.get("remote")),
    posted: pick(TIME_RANGES, params.get("posted")),
    analyzed: pick(TIME_RANGES, params.get("analyzed")),
    score: pick(SCORE_FLOORS, params.get("score")),
    where: pick(WHERE_CHOICES, params.get("where")),
  };
}

/// activeFilterCount is how many of view's filters are set.
export function activeFilterCount(filters: JobFilters, view: JobsView): number {
  return VIEW_FILTERS[view].filter((key) => filters[key] !== "").length;
}

/// since is the start of range, counted back from now.
export function since(range: TimeRange, now = new Date()): Date {
  return new Date(now.getTime() - TIME_RANGES[range].days * 24 * 60 * 60 * 1000);
}

/// matchPasses applies the Analyzed view's filters to one match and its job.
/// A job not loaded yet cannot be judged by its source or remote type, so it
/// is left out until it is.
export function matchPasses(match: JobMatch, job: Job | undefined, filters: JobFilters, now = new Date()): boolean {
  if (filters.score && match.score < Number(filters.score)) return false;
  if (filters.analyzed && new Date(match.created_at) < since(filters.analyzed, now)) return false;
  if (filters.source || filters.remote) {
    if (!job) return false;
    if (filters.source && job.source_id !== filters.source) return false;
    if (filters.remote && job.remote_type?.toLowerCase() !== filters.remote) return false;
  }
  return true;
}
