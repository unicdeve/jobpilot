// A port of the Go core's search rule (internal/core/domain/job.go, SearchCriteria.MatchesFor), ADR-0037.
// It has no Exact, network or storage dependency, so it runs the same in the app's JS engine and under `bun test`.
// Includes work-from/relocation (workableFor) and fit-hours (hoursFor), which read the job's precomputed signals (workable, relocation, visaSponsorship, hours).
// A job with no signals set is treated as not yet read: it is kept, as in Go.

import { noHours, shiftFor, type WorkingHours } from './hours.ts';
import { offerAvailable, type Offer } from './offers.ts';
import { openTo, unknownWorkable, workableKnown, type Workable } from './workable.ts';

export interface Job {
  title: string;
  companyName: string;
  descriptionMarkdown: string;
  locations: string[];
  remoteType: string;
  employmentType: string;
  salaryMin: number;
  salaryMax: number;
  // Read from the job by the signals reader (ADR-0023, ADR-0025); absent until it has been.
  workable?: Workable;
  relocation?: Offer;
  visaSponsorship?: Offer;
  hours?: WorkingHours;
}

// What a search needs to know about the candidate beyond the criteria (Go's Seeker).
export interface Seeker {
  // ISO codes of the countries the candidate may work in without sponsorship.
  authorizedCountries: string[];
  // The candidate's time zone's standard offset, when hasTimeZone.
  utcOffset: number;
  hasTimeZone: boolean;
}

export const noSeeker: Seeker = { authorizedCountries: [], utcOffset: 0, hasTimeZone: false };

export interface SearchCriteria {
  keywords: string;
  title: string;
  location: string;
  remotePreference: string;
  employmentType: string;
  salaryMin: number;
  salaryMax: number;
  excludedCompanies: string[];
  // ISO code of the country the candidate works from, or empty for no preference (ADR-0023 section 4).
  workFrom?: string;
  // Also keeps jobs elsewhere that offer or mention relocation or visa sponsorship. Matters only with workFrom set.
  includeRelocation?: boolean;
  // Leaves out jobs that require hours the candidate cannot work, moving their day by up to hoursShift hours (ADR-0025 section 3).
  fitHours?: boolean;
  hoursShift?: number;
}

// The most a candidate can say they would move their day.
export const maxHoursShift = 12;

export const emptyCriteria: SearchCriteria = {
  keywords: '',
  title: '',
  location: '',
  remotePreference: '',
  employmentType: '',
  salaryMin: 0,
  salaryMax: 0,
  excludedCompanies: [],
  workFrom: '',
  includeRelocation: false,
  fitHours: false,
  hoursShift: 0,
};

// strings.Fields(strings.ToLower(strings.TrimSpace(field)))
export const searchTerms = (field: string): string[] =>
  field
    .trim()
    .toLowerCase()
    .split(/\s+/)
    .filter((t) => t !== '');

const matchesAllTerms = (field: string, text: string): boolean => {
  const haystack = text.toLowerCase();
  return searchTerms(field).every((term) => haystack.includes(term));
};

const equalsFold = (criterion: string, value: string): boolean => {
  const c = criterion.trim();
  return c === '' || c.toLowerCase() === value.trim().toLowerCase();
};

// A job with no salary is kept: most boards publish none.
const withinSalary = (c: SearchCriteria, job: Job): boolean => {
  if (job.salaryMin === 0 && job.salaryMax === 0) return true;
  if (c.salaryMin > 0 && Math.max(job.salaryMin, job.salaryMax) < c.salaryMin) return false;
  if (c.salaryMax > 0 && job.salaryMin > c.salaryMax) return false;
  return true;
};

const excludes = (c: SearchCriteria, job: Job): boolean =>
  c.excludedCompanies.some((x) => x.trim().toLowerCase() === job.companyName.trim().toLowerCase());

// A job that does not say where it can be done from is kept; only one that says it is open elsewhere, and does not help the candidate get there, is left out.
const workableFor = (c: SearchCriteria, job: Job, authorized: string[]): boolean => {
  const from = (c.workFrom ?? '').trim();
  const workable = job.workable ?? unknownWorkable;
  if (from === '' || !workableKnown(workable) || openTo(workable, from)) return true;
  if (!c.includeRelocation) return false;
  const relocation = job.relocation;
  const visa = job.visaSponsorship;
  if (!(relocation && offerAvailable(relocation)) && !(visa && offerAvailable(visa))) return false;
  // A job that refuses sponsorship is one only a candidate already allowed to work there can take (ADR-0023 section 5).
  if (visa?.status === 'declined') return authorized.some((country) => openTo(workable, country));
  return true;
};

// Only a requirement can leave a job out: one that says nothing about its hours, or only prefers some, is kept.
const hoursFor = (c: SearchCriteria, job: Job, seeker: Seeker): boolean => {
  const hours = job.hours ?? noHours;
  if (!c.fitHours || !seeker.hasTimeZone || hours.status !== 'required') return true;
  return shiftFor(hours, seeker.utcOffset) <= (c.hoursShift ?? 0);
};

// Whether the criteria would match every job in existence. fitHours does not count.
export const isEmpty = (c: SearchCriteria): boolean =>
  c.keywords.trim() === '' &&
  c.title.trim() === '' &&
  c.location.trim() === '' &&
  c.remotePreference.trim() === '' &&
  c.employmentType.trim() === '' &&
  c.salaryMin === 0 &&
  c.salaryMax === 0 &&
  c.excludedCompanies.length === 0 &&
  (c.workFrom ?? '').trim() === '';

// Whether the job could match once its description is read: every criterion a description cannot change.
export const mayMatch = (c: SearchCriteria, job: Job): boolean =>
  matchesAllTerms(c.title, job.title) &&
  matchesAllTerms(c.location, job.locations.join(' ')) &&
  equalsFold(c.remotePreference, job.remoteType) &&
  equalsFold(c.employmentType, job.employmentType) &&
  withinSalary(c, job) &&
  !excludes(c, job);

export function matchesFor(c: SearchCriteria, job: Job, seeker: Seeker): boolean {
  return (
    matchesAllTerms(c.keywords, `${job.title} ${job.descriptionMarkdown}`) &&
    matchesAllTerms(c.title, job.title) &&
    matchesAllTerms(c.location, job.locations.join(' ')) &&
    equalsFold(c.remotePreference, job.remoteType) &&
    equalsFold(c.employmentType, job.employmentType) &&
    withinSalary(c, job) &&
    workableFor(c, job, seeker.authorizedCountries) &&
    hoursFor(c, job, seeker) &&
    !excludes(c, job)
  );
}

// For a candidate with no work authorization or time zone recorded.
export const matches = (c: SearchCriteria, job: Job): boolean => matchesFor(c, job, noSeeker);
