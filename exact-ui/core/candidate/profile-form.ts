// The profile as plain text, so a view can hold and edit it (ADR-0037). The candidate types every fact: nothing here fills a
// field in, guesses a date or invents a skill (AGENTS.md principle 6). Lists are one entry per line and are read back strictly:
// a line that cannot be read is an error naming the line, never a guess. Pure: no storage, no clock.
import { dateString, parseDraftDate } from '../shared/date.ts';
import { authorizedCountries, isLinkKind, linkKinds, locationDisplay, fullName, period, utcOffset, type Candidate, type CandidateExperience, type CandidateLink, type CandidateSkill, type WorkAuthorization } from './candidate.ts';
import { normalizeDraftUrl } from './profile-draft.ts';

export interface ProfileForm {
  firstName: string; lastName: string; email: string; phone: string;
  city: string; region: string; country: string; timeZone: string;
  // One per line: `github https://github.com/you`.
  links: string;
  // One per line: `Nigeria: authorized` or `Germany: needs sponsorship`.
  authorizations: string;
  // One per line or comma-separated: `Go: 6` (years) or `Go`.
  skills: string;
  // One per line: `Title | Company | Start | End | What you did`, an empty or `present` End for a current role.
  experiences: string;
}

export const emptyProfileForm: ProfileForm = {
  firstName: '', lastName: '', email: '', phone: '', city: '', region: '', country: '', timeZone: '',
  links: '', authorizations: '', skills: '', experiences: '',
};

const oneLine = (s: string): string => s.replace(/\s*[\r\n]+\s*/g, ' ').trim();
const years = (n: number | null): string => (n === null ? '' : `: ${n}`);

export function profileToForm(c: Candidate | undefined): ProfileForm {
  if (c === undefined) return emptyProfileForm;
  return {
    firstName: c.firstName, lastName: c.lastName, email: c.email, phone: c.phone,
    city: c.locationCity, region: c.locationRegion, country: c.locationCountry, timeZone: c.timeZone,
    links: c.links.map((l) => `${l.kind} ${l.url}`).join('\n'),
    authorizations: c.workAuthorizations.map((a) => `${a.country}: ${a.isAuthorized && !a.requiresSponsorship ? 'authorized' : 'needs sponsorship'}`).join('\n'),
    skills: c.skills.map((s) => `${s.name}${years(s.yearsExperience)}`).join('\n'),
    experiences: c.experiences.map((e) => [e.title, e.company, e.startDate ? dateString(e.startDate) : '', e.isCurrent ? 'present' : e.endDate ? dateString(e.endDate) : '', oneLine(e.description)]
      .join(' | ').replace(/( \| )+$/, '')).join('\n'),
  };
}

const lines = (text: string): { n: number; text: string }[] =>
  text.split(/\r?\n/).map((t, i) => ({ n: i + 1, text: t.trim() })).filter((l) => l.text !== '');

export type Parsed<T> = { ok: true; value: T } | { ok: false; error: string };
const bad = (error: string): { ok: false; error: string } => ({ ok: false, error });

export function parseLinks(text: string): Parsed<CandidateLink[]> {
  const out: CandidateLink[] = [];
  for (const l of lines(text)) {
    const space = l.text.search(/\s/);
    const kind = (space < 0 ? l.text : l.text.slice(0, space)).toLowerCase();
    if (!isLinkKind(kind)) return bad(`Links, line ${l.n}: start with the kind of link (${linkKinds.join(', ')}), then the address.`);
    const url = space < 0 ? undefined : normalizeDraftUrl(l.text.slice(space));
    if (url === undefined) return bad(`Links, line ${l.n}: "${space < 0 ? '' : l.text.slice(space).trim()}" is not a web address.`);
    out.push({ id: '', kind, url });
  }
  return { ok: true, value: out };
}

export function parseAuthorizations(text: string): Parsed<WorkAuthorization[]> {
  const out: WorkAuthorization[] = [];
  for (const l of lines(text)) {
    const colon = l.text.lastIndexOf(':');
    const country = colon < 0 ? '' : l.text.slice(0, colon).trim();
    const status = colon < 0 ? '' : l.text.slice(colon + 1).trim().toLowerCase();
    if (country === '' || (status !== 'authorized' && status !== 'needs sponsorship'))
      return bad(`Work authorization, line ${l.n}: write the country, a colon, then "authorized" or "needs sponsorship".`);
    out.push({ id: '', country, isAuthorized: status === 'authorized', requiresSponsorship: status === 'needs sponsorship' });
  }
  return { ok: true, value: out };
}

export function parseSkills(text: string): Parsed<CandidateSkill[]> {
  const out: CandidateSkill[] = [];
  let n = 0;
  for (const raw of text.split(/\r?\n/)) {
    n++;
    for (const part of raw.split(',')) {
      const entry = part.trim();
      if (entry === '') continue;
      const colon = entry.lastIndexOf(':');
      const name = (colon < 0 ? entry : entry.slice(0, colon)).trim();
      const given = colon < 0 ? '' : entry.slice(colon + 1).trim();
      if (name === '') return bad(`Skills, line ${n}: a skill needs a name.`);
      if (given !== '' && !/^\d{1,2}(\.\d)?$/.test(given)) return bad(`Skills, line ${n}: the years for ${name} must be a number, such as ${name}: 5.`);
      out.push({ id: '', name, yearsExperience: given === '' ? null : Number(given) });
    }
  }
  return { ok: true, value: out };
}

export function parseExperiences(text: string): Parsed<CandidateExperience[]> {
  const out: CandidateExperience[] = [];
  for (const l of lines(text)) {
    const [title = '', company = '', start = '', end = '', ...rest] = l.text.split('|').map((p) => p.trim());
    if (title === '' && company === '') return bad(`Experience, line ${l.n}: give at least a title or a company.`);
    const startDate = parseDraftDate(start);
    if (start !== '' && startDate === null) return bad(`Experience, line ${l.n}: "${start}" is not a date I can read; write it as 2022-07 or July 2022.`);
    const present = end === '' || /^(present|current|now)$/i.test(end);
    const endDate = present ? null : parseDraftDate(end);
    if (!present && endDate === null) return bad(`Experience, line ${l.n}: "${end}" is not a date I can read; write it as 2023-01 or January 2023, or leave it empty for a current role.`);
    // A line with no dates at all is a role whose dates were not given, not a current one.
    const isCurrent = present && startDate !== null;
    out.push({ id: '', company, title, startDate, endDate, isCurrent, description: rest.join(' | ').trim() });
  }
  return { ok: true, value: out };
}

// The candidate with the form's fields applied over `base`, which keeps what the form does not hold (notice period, preference).
export function profileFromForm(f: ProfileForm, base: Candidate): Parsed<Candidate> {
  const links = parseLinks(f.links);
  if (!links.ok) return links;
  const auths = parseAuthorizations(f.authorizations);
  if (!auths.ok) return auths;
  const skills = parseSkills(f.skills);
  if (!skills.ok) return skills;
  const experiences = parseExperiences(f.experiences);
  if (!experiences.ok) return experiences;
  const email = f.email.trim();
  if (email !== '' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return bad(`"${email}" is not an email address.`);
  return { ok: true, value: {
    ...base, firstName: f.firstName, lastName: f.lastName, email, phone: f.phone,
    locationCity: f.city, locationRegion: f.region, locationCountry: f.country, timeZone: f.timeZone,
    links: links.value, workAuthorizations: auths.value, skills: skills.value, experiences: experiences.value,
  } };
}

// What the Profile screen shows, one line each.
export interface ProfileView {
  name: string; email: string; phone: string; location: string; timeZone: string;
  links: string[]; authorizations: string[]; skills: string; experiences: string[];
}
export function describeProfile(c: Candidate): ProfileView {
  return {
    name: fullName(c), email: c.email, phone: c.phone, location: locationDisplay(c), timeZone: c.timeZone,
    links: c.links.map((l) => `${l.kind}: ${l.url}`),
    authorizations: c.workAuthorizations.map((a) => `${a.country}: ${a.isAuthorized && !a.requiresSponsorship ? 'authorized' : 'needs sponsorship'}`),
    skills: c.skills.map((s) => (s.yearsExperience === null ? s.name : `${s.name} (${s.yearsExperience} y)`)).join(', '),
    experiences: c.experiences.map((e) => [e.title, e.company].filter((p) => p !== '').join(' at ') + (period(e) === '' ? '' : ` · ${period(e)}`)),
  };
}

// The Seeker a search reads: where the candidate may work without sponsorship, and their time zone (core/search/criteria.ts).
export function seekerOf(c: Candidate | undefined): { authorizedCountries: string[]; utcOffset: number; hasTimeZone: boolean } {
  if (c === undefined) return { authorizedCountries: [], utcOffset: 0, hasTimeZone: false };
  const zone = utcOffset(c);
  return { authorizedCountries: authorizedCountries(c), utcOffset: zone.offset, hasTimeZone: zone.ok };
}
