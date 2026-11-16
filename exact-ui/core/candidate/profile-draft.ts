// The pure half of the Go core's ProfileDraft (internal/core/domain/profile_draft.go), ADR-0037.
// A draft is a proposal read off a resume for the user to review: it is never written to the profile directly (ADR-0011).
// It never proposes work authorization. What reads a resume into a draft needs an LLM and a converter and is not here.
import { isLinkKind, type CandidateExperience, type CandidateLink, type CandidateSkill } from './candidate.ts';
import { parseDraftDate } from '../shared/date.ts';

export interface DraftSkill { name: string; yearsExperience: number | null }
export interface DraftExperience { company: string; title: string; startDate: string; endDate: string; isCurrent: boolean; description: string }
export interface DraftLink { kind: string; url: string }
export interface ProfileDraft {
  firstName: string; lastName: string; email: string; phone: string;
  locationCity: string; locationRegion: string; locationCountry: string;
  summary: string; skills: DraftSkill[]; experiences: DraftExperience[]; links: DraftLink[];
}

const PRESENT = new Set(['present', 'current', 'now', 'ongoing', 'to date', 'presently']);
const isPresent = (v: string): boolean => PRESENT.has(v.trim().toLowerCase());

// A skill listed twice is one row; unnamed ones are dropped. An unstated duration stays unstated, not zero.
export function proposedSkills(d: ProfileDraft): Omit<CandidateSkill, 'id'>[] {
  const seen = new Set<string>();
  const out: Omit<CandidateSkill, 'id'>[] = [];
  for (const s of d.skills) {
    const name = s.name.trim();
    const key = name.toLowerCase();
    if (name === '' || seen.has(key)) continue;
    seen.add(key);
    out.push({ name, yearsExperience: s.yearsExperience });
  }
  return out;
}

// An entry naming neither a company nor a title is dropped; an unreadable date is dropped rather than guessed.
// "Present" as an end date means the role is current, and a current role has no end date.
export function proposedExperiences(d: ProfileDraft): Omit<CandidateExperience, 'id'>[] {
  const out: Omit<CandidateExperience, 'id'>[] = [];
  for (const p of d.experiences) {
    const company = p.company.trim();
    const title = p.title.trim();
    if (company === '' && title === '') continue;
    let isCurrent = p.isCurrent;
    let endDate = parseDraftDate(p.endDate);
    if (endDate === null && isPresent(p.endDate)) isCurrent = true;
    if (isCurrent) endDate = null;
    out.push({ company, title, startDate: parseDraftDate(p.startDate), endDate, isCurrent, description: p.description.trim() });
  }
  return out;
}

const hasScheme = (raw: string): boolean => {
  for (let i = 0; i < raw.length; i++) {
    const c = raw[i]!;
    if (c === ':') return i > 0;
    if (c === '.' || c === '/') return false;
    if (/[A-Za-z]/.test(c)) continue;
    if (/[0-9+-]/.test(c)) { if (i === 0) return false; continue; }
    return false;
  }
  return false;
};

// A usable URL, or undefined for a value that is not one. A word is not a URL however confident the model was;
// a bare host is given https.
export function normalizeDraftUrl(input: string): string | undefined {
  const raw = input.trim();
  if (raw === '' || /[ \t\n\r]/.test(raw)) return undefined;
  let rest = raw;
  let hadScheme = false;
  for (const scheme of ['https://', 'http://']) {
    if (rest.length >= scheme.length && rest.slice(0, scheme.length).toLowerCase() === scheme) { rest = rest.slice(scheme.length); hadScheme = true; break; }
  }
  if (!hadScheme && hasScheme(raw)) return undefined;
  let host = rest;
  const cut = host.search(/[/?#]/);
  if (cut >= 0) host = host.slice(0, cut);
  if (host.endsWith('.')) host = host.slice(0, -1);
  if (!host.includes('.')) return undefined;
  if (host.split('.').some((l) => l === '')) return undefined;
  return hadScheme ? raw : `https://${raw}`;
}

const identifiesAProfile = (kind: string): boolean => kind === 'linkedin' || kind === 'github' || kind === 'twitter';

const hasPath = (url: string): boolean => {
  let rest = url;
  const i = rest.indexOf('://');
  if (i >= 0) rest = rest.slice(i + 3);
  const slash = rest.indexOf('/');
  return slash >= 0 && rest.slice(slash).replace(/^\/+|\/+$/g, '') !== '';
};

// Only kinds the profile has, the first of each, and for a profile kind only a URL that names a person (the site itself is not one).
export function proposedLinks(d: ProfileDraft): Omit<CandidateLink, 'id'>[] {
  const seen = new Set<string>();
  const out: Omit<CandidateLink, 'id'>[] = [];
  for (const p of d.links) {
    const kind = p.kind.trim().toLowerCase();
    const url = normalizeDraftUrl(p.url);
    if (url === undefined || !isLinkKind(kind) || seen.has(kind)) continue;
    if (identifiesAProfile(kind) && !hasPath(url)) continue;
    seen.add(kind);
    out.push({ kind, url });
  }
  return out;
}
