import { expect, test } from 'bun:test';
import {
  authorizedCountries,
  currentExperience,
  emptyCandidate,
  fullName,
  locationDisplay,
  normalizeTimeZone,
  period,
  renderExperiences,
  renderSkills,
  skillNamed,
  totalYearsOfExperience,
  type Candidate,
  type CandidateExperience,
} from './candidate.ts';
import { newDate } from '../shared/date.ts';
import {
  normalizeDraftUrl,
  proposedExperiences,
  proposedLinks,
  proposedSkills,
  type ProfileDraft,
} from './profile-draft.ts';

const d = (y: number, m: number, day: number) => newDate(y, m, day);
const exp = (o: Partial<CandidateExperience>): CandidateExperience => ({
  id: '',
  company: '',
  title: '',
  startDate: null,
  endDate: null,
  isCurrent: false,
  description: '',
  ...o,
});
const withExp = (experiences: CandidateExperience[]): Candidate => ({
  ...emptyCandidate('c', 0),
  experiences,
});

// The cases are the Go tests' (internal/core/domain/candidate_test.go, profile_draft_test.go).
test('current experience: the flag wins, else the open-ended latest start', () => {
  expect(
    currentExperience(
      withExp([
        exp({ company: 'Old Co', startDate: d(2019, 1, 1), endDate: d(2022, 6, 30) }),
        exp({ company: 'Acme', startDate: d(2022, 7, 1), isCurrent: true }),
      ]),
    )?.company,
  ).toBe('Acme');
  expect(
    currentExperience(
      withExp([
        exp({ company: 'Old Co', startDate: d(2015, 1, 1) }),
        exp({ company: 'Acme', startDate: d(2022, 7, 1) }),
        exp({ company: 'Finished', startDate: d(2023, 1, 1), endDate: d(2024, 1, 1) }),
      ]),
    )?.company,
  ).toBe('Acme');
  expect(
    currentExperience(withExp([exp({ startDate: d(2015, 1, 1), endDate: d(2020, 1, 1) })])),
  ).toBeUndefined();
  expect(currentExperience(withExp([]))).toBeUndefined();
});

test('total years of experience is the span, never invented', () => {
  const now = Date.UTC(2026, 8, 21);
  const years = (e: CandidateExperience[]) => totalYearsOfExperience(withExp(e), now);
  expect(
    years([
      exp({ startDate: d(2016, 9, 21), endDate: d(2022, 6, 30) }),
      exp({ startDate: d(2022, 7, 1), isCurrent: true }),
    ]),
  ).toBeCloseTo(10, 1);
  expect(
    years([
      exp({ startDate: d(2022, 9, 21), endDate: d(2026, 9, 21) }),
      exp({ startDate: d(2022, 9, 21), endDate: d(2026, 9, 21) }),
    ]),
  ).toBeCloseTo(4, 1);
  expect(years([exp({ startDate: d(2010, 9, 21), endDate: d(2015, 9, 21) })])).toBeCloseTo(5, 1);
  expect(years([exp({ company: 'Acme', title: 'Engineer' })])).toBeUndefined();
  expect(years([])).toBeUndefined();
});

test('period and rendering', () => {
  expect(period(exp({ startDate: d(2019, 3, 1), endDate: d(2022, 6, 1) }))).toBe(
    'Mar 2019 \u2013 Jun 2022',
  );
  expect(period(exp({ startDate: d(2022, 7, 1), isCurrent: true }))).toBe(
    'Jul 2022 \u2013 present',
  );
  expect(period(exp({ startDate: d(2022, 7, 1) }))).toBe('Jul 2022 \u2013 present');
  expect(period(exp({ endDate: d(2022, 6, 1) }))).toBe('until Jun 2022');
  expect(period(exp({ company: 'Acme' }))).toBe('');
  const out = renderExperiences(
    withExp([
      exp({
        company: 'Acme',
        title: 'Staff Engineer',
        startDate: d(2022, 7, 1),
        isCurrent: true,
        description: 'Led the payments rewrite.',
      }),
      exp({
        company: 'Old Co',
        title: 'Engineer',
        startDate: d(2019, 3, 1),
        endDate: d(2022, 6, 1),
      }),
    ]),
  );
  expect(out).toContain('- Staff Engineer at Acme (Jul 2022 \u2013 present)');
  expect(out).toContain('  Led the payments rewrite.');
  expect(out).toContain('- Engineer at Old Co (Mar 2019 \u2013 Jun 2022)');
});

test('skills, name and location', () => {
  const c: Candidate = {
    ...emptyCandidate('c', 0),
    firstName: ' Ada',
    lastName: 'Lovelace ',
    locationCity: 'Lagos',
    locationRegion: ' ',
    locationCountry: 'Nigeria',
    skills: [
      { id: 'a', name: 'Go', yearsExperience: 4 },
      { id: 'b', name: 'TypeScript', yearsExperience: null },
    ],
  };
  expect(renderSkills(c)).toBe('Go, TypeScript');
  expect(skillNamed(c, 'go')?.yearsExperience).toBe(4);
  expect(skillNamed(c, 'Rust')).toBeUndefined();
  expect(fullName(c)).toBe('Ada Lovelace');
  expect(locationDisplay(c)).toBe('Lagos, Nigeria');
});

test('authorized countries leave out sponsorship and unplaceable names', () => {
  const c: Candidate = {
    ...emptyCandidate('c', 0),
    workAuthorizations: [
      { id: '1', country: 'Germany', isAuthorized: true, requiresSponsorship: false },
      { id: '2', country: 'United Kingdom', isAuthorized: true, requiresSponsorship: true },
      { id: '3', country: 'Atlantis', isAuthorized: true, requiresSponsorship: false },
      { id: '4', country: 'DE', isAuthorized: true, requiresSponsorship: false },
    ],
  };
  expect(authorizedCountries(c)).toEqual(['DE']);
});

test('time zone', () => {
  expect(normalizeTimeZone('  ')).toEqual({ ok: true, zone: '' });
  expect(normalizeTimeZone('africa/lagos')).toEqual({ ok: true, zone: 'Africa/Lagos' });
  expect(normalizeTimeZone('Mars/Base').ok).toBe(false);
});

const draft = (o: Partial<ProfileDraft>): ProfileDraft => ({
  firstName: '',
  lastName: '',
  email: '',
  phone: '',
  locationCity: '',
  locationRegion: '',
  locationCountry: '',
  summary: '',
  skills: [],
  experiences: [],
  links: [],
  ...o,
});

test('proposed skills', () => {
  const got = proposedSkills(
    draft({
      skills: [
        { name: 'Go', yearsExperience: 4 },
        { name: '  TypeScript  ', yearsExperience: null },
        { name: 'go', yearsExperience: 4 },
        { name: '   ', yearsExperience: null },
      ],
    }),
  );
  expect(got.map((s) => s.name)).toEqual(['Go', 'TypeScript']);
  expect(got[1]!.yearsExperience).toBeNull();
});

test('proposed experiences', () => {
  const e = (o: Partial<ProfileDraft['experiences'][number]>) => ({
    company: '',
    title: '',
    startDate: '',
    endDate: '',
    isCurrent: false,
    description: '',
    ...o,
  });
  const got = proposedExperiences(
    draft({
      experiences: [
        e({ company: 'Acme', title: 'Staff Engineer', startDate: '2022-07', isCurrent: true }),
        e({ company: 'Old Co', title: 'Engineer', startDate: '2016-09-21', endDate: '2022-06' }),
        e({ company: 'Acme', title: 'Engineer', startDate: '2022-07', endDate: 'Present' }),
        e({ company: 'Acme', title: 'Engineer', startDate: 'sometime in the spring' }),
        e({ description: 'Did some things' }),
      ],
    }),
  );
  expect(got.length).toBe(4);
  expect(got[0]!.startDate).toEqual(d(2022, 7, 1));
  expect(got[0]!.endDate).toBeNull();
  expect(got[1]!.endDate).not.toBeNull();
  expect(got[2]!.isCurrent).toBe(true);
  expect(got[2]!.endDate).toBeNull();
  expect(got[3]!.startDate).toBeNull();
});

test('proposed links', () => {
  expect(
    proposedLinks(
      draft({
        links: [
          { kind: 'LinkedIn', url: 'https://linkedin.com/in/ada' },
          { kind: 'facebook', url: 'https://facebook.com/ada' },
          { kind: 'github', url: '' },
          { kind: 'linkedin', url: 'https://linkedin.com/in/someone-else' },
        ],
      }),
    ),
  ).toEqual([{ kind: 'linkedin', url: 'https://linkedin.com/in/ada' }]);
  expect(
    proposedLinks(
      draft({
        links: [
          { kind: 'github', url: 'ub' },
          { kind: 'linkedin', url: '' },
          { kind: 'website', url: 'Blog' },
        ],
      }),
    ),
  ).toEqual([]);
  expect(
    proposedLinks(
      draft({
        links: [
          { kind: 'github', url: 'github.com' },
          { kind: 'linkedin', url: 'https://linkedin.com' },
          { kind: 'twitter', url: 'twitter.com/' },
          { kind: 'website', url: 'ada.dev' },
        ],
      }),
    ),
  ).toEqual([{ kind: 'website', url: 'https://ada.dev' }]);
});

test('normalize draft url', () => {
  expect(normalizeDraftUrl('https://github.com/ada')).toBe('https://github.com/ada');
  expect(normalizeDraftUrl('http://ada.dev')).toBe('http://ada.dev');
  expect(normalizeDraftUrl('linkedin.com/in/ada')).toBe('https://linkedin.com/in/ada');
  expect(normalizeDraftUrl('www.ada.dev/blog?x=1')).toBe('https://www.ada.dev/blog?x=1');
  for (const bad of [
    'ub',
    'Blog',
    'LinkedIn',
    '',
    '   ',
    'my portfolio site',
    'mailto:ada@example.com',
    'javascript:alert(1)',
    'https://',
  ])
    expect(normalizeDraftUrl(bad)).toBeUndefined();
});
