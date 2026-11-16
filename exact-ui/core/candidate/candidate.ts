// A port of the Go core's candidate profile (internal/core/domain/candidate.go, ProfileService's time-zone rule), ADR-0037.
// The profile is only what the user entered: nothing here fills a field in or invents a fact (AGENTS.md principle 6).
// Pure: no storage, no clock (`now` is passed in), so it runs the same in the app's JS engine and under `bun test`.
import { dateAfter, type DateValue } from '../shared/date.ts';
import { resolveCountry, timeZoneByID } from '../search/geo.ts';

export type LinkKind = 'linkedin' | 'github' | 'portfolio' | 'website' | 'twitter' | 'other';

// Forms ask for links one at a time, so each kind is stored on its own.
export const linkKinds: readonly LinkKind[] = [
  'linkedin',
  'github',
  'portfolio',
  'website',
  'twitter',
  'other',
];
export const isLinkKind = (kind: string): kind is LinkKind =>
  (linkKinds as readonly string[]).includes(kind);

export type RemotePreference = '' | 'remote' | 'hybrid' | 'onsite';
export const remotePreferences: readonly RemotePreference[] = ['remote', 'hybrid', 'onsite'];
// The empty value is valid: not having said is a normal state.
export const isRemotePreference = (p: string): p is RemotePreference =>
  p === '' || (remotePreferences as readonly string[]).includes(p);

export interface CandidateLink {
  id: string;
  kind: LinkKind;
  url: string;
}
// Per country, because the question always is.
export interface WorkAuthorization {
  id: string;
  country: string;
  isAuthorized: boolean;
  requiresSponsorship: boolean;
}
export interface CandidateExperience {
  id: string;
  company: string;
  title: string;
  startDate: DateValue | null;
  endDate: DateValue | null;
  isCurrent: boolean;
  description: string;
}
export interface CandidateSkill {
  id: string;
  name: string;
  yearsExperience: number | null;
}

export interface Candidate {
  id: string;
  firstName: string;
  lastName: string;
  email: string;
  phone: string;
  locationCity: string;
  locationRegion: string;
  locationCountry: string;
  // The IANA zone, such as Africa/Lagos, or empty.
  timeZone: string;
  // Null is unsaid; zero is an answer ("immediately").
  noticePeriodDays: number | null;
  earliestStartDate: DateValue | null;
  summary: string;
  remotePreference: RemotePreference;
  links: CandidateLink[];
  workAuthorizations: WorkAuthorization[];
  skills: CandidateSkill[];
  experiences: CandidateExperience[];
  // Epoch milliseconds.
  createdAt: number;
  updatedAt: number;
}

export const linkOf = (c: Candidate, kind: LinkKind): CandidateLink | undefined =>
  c.links.find((l) => l.kind === kind);

// The ISO codes of the countries the candidate may work in without sponsorship; a country that cannot be placed is left out.
export function authorizedCountries(c: Candidate): string[] {
  const codes: string[] = [];
  for (const a of c.workAuthorizations) {
    if (!a.isAuthorized || a.requiresSponsorship) continue;
    const code = resolveCountry(a.country);
    if (code !== undefined && !codes.includes(code)) codes.push(code);
  }
  return codes;
}

// The zone's offset from UTC in hours without daylight saving, and whether JobPilot knows the zone.
export function utcOffset(c: Candidate): { offset: number; ok: boolean } {
  const zone = timeZoneByID(c.timeZone);
  return { offset: zone?.offset ?? 0, ok: zone !== undefined };
}

export const locationDisplay = (c: Candidate): string =>
  [c.locationCity, c.locationRegion, c.locationCountry]
    .map((p) => p.trim())
    .filter((p) => p !== '')
    .join(', ');

export const fullName = (c: Candidate): string => `${c.firstName} ${c.lastName}`.trim();

// What a form means by "Current company": the flag wins, else the open-ended job with the latest start.
export function currentExperience(c: Candidate): CandidateExperience | undefined {
  const flagged = c.experiences.find((e) => e.isCurrent);
  if (flagged) return flagged;
  let best: CandidateExperience | undefined;
  for (const e of c.experiences) {
    if (e.endDate !== null || e.startDate === null) continue;
    if (best === undefined || dateAfter(e.startDate, best.startDate!)) best = e;
  }
  return best;
}

const utc = (d: DateValue): number => Date.UTC(d.year, d.month - 1, d.day);

// The span from the earliest start to the latest end (or `now` while a job is current), so overlapping jobs count once.
// Undefined when no entry has a start date: a number invented from nothing must not reach a form.
export function totalYearsOfExperience(c: Candidate, now: number): number | undefined {
  let earliest: number | undefined;
  let latest: number | undefined;
  for (const e of c.experiences) {
    if (e.startDate === null) continue;
    const start = utc(e.startDate);
    if (earliest === undefined || start < earliest) earliest = start;
    const end = !e.isCurrent && e.endDate !== null ? utc(e.endDate) : now;
    if (latest === undefined || end > latest) latest = end;
  }
  if (earliest === undefined || latest === undefined || latest <= earliest) return undefined;
  return (latest - earliest) / 3_600_000 / (24 * 365.25);
}

export const skillNamed = (c: Candidate, name: string): CandidateSkill | undefined =>
  c.skills.find((s) => s.name.trim().toLowerCase() === name.trim().toLowerCase());

export const skillNames = (c: Candidate): string[] =>
  c.skills.map((s) => s.name.trim()).filter((n) => n !== '');

export const renderSkills = (c: Candidate): string => skillNames(c).join(', ');

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const monthYear = (d: DateValue): string => `${MONTHS[d.month - 1]} ${d.year}`;

// An experience's dates the way a CV does.
export function period(e: CandidateExperience): string {
  if (e.startDate === null && e.endDate === null) return '';
  if (e.startDate === null) return `until ${monthYear(e.endDate!)}`;
  if (e.isCurrent || e.endDate === null) return `${monthYear(e.startDate)} – present`;
  return `${monthYear(e.startDate)} – ${monthYear(e.endDate)}`;
}

export function renderExperiences(c: Candidate): string {
  let out = '';
  for (const e of c.experiences) {
    out += `- ${e.title.trim()}`;
    if (e.company !== '') out += ` at ${e.company.trim()}`;
    const p = period(e);
    if (p !== '') out += ` (${p})`;
    out += '\n';
    if (e.description.trim() !== '') out += `  ${e.description.trim()}\n`;
  }
  return out.trim();
}

// A zone as GeoNames writes it, or an error for one JobPilot does not list; empty stays empty.
export function normalizeTimeZone(
  name: string,
): { ok: true; zone: string } | { ok: false; error: string } {
  const trimmed = name.trim();
  if (trimmed === '') return { ok: true, zone: '' };
  const zone = timeZoneByID(trimmed);
  if (!zone)
    return {
      ok: false,
      error: `"${trimmed}" is not a time zone JobPilot knows; use its IANA name, such as Africa/Lagos`,
    };
  return { ok: true, zone: zone.id };
}

export const emptyCandidate = (id: string, now: number): Candidate => ({
  id,
  firstName: '',
  lastName: '',
  email: '',
  phone: '',
  locationCity: '',
  locationRegion: '',
  locationCountry: '',
  timeZone: '',
  noticePeriodDays: null,
  earliestStartDate: null,
  summary: '',
  remotePreference: '',
  links: [],
  workAuthorizations: [],
  skills: [],
  experiences: [],
  createdAt: now,
  updatedAt: now,
});
