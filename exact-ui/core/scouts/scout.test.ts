import { expect, test } from 'bun:test';
import { emptyCriteria } from '../search/criteria.ts';
import {
  eligible,
  isValidSource,
  preScreen,
  validateScout,
  type EligibilityInput,
  type Scout,
} from './scout.ts';

// The cases are the Go tests' (internal/core/domain/scout_test.go), so the two cores cannot drift silently.
const off = { kind: 'off' as const, everyHours: 0, dailyAt: 0 };
const scout = (adjust: Partial<Scout> = {}): Scout => ({
  name: 'Daily Go',
  savedSearchId: 'search-1',
  minMatchScore: 70,
  maxApplicationsPerRun: 5,
  source: 'local',
  isEnabled: true,
  schedule: off,
  ...adjust,
});
const job = (title: string, extra = {}) => ({
  title,
  companyName: 'Acme',
  descriptionMarkdown: '',
  locations: [],
  remoteType: '',
  employmentType: '',
  salaryMin: 0,
  salaryMax: 0,
  ...extra,
});
const goJob = job('Senior Go Engineer');
const criteria = { ...emptyCriteria, keywords: 'Go' };
const input = (extra: Partial<EligibilityInput> = {}): EligibilityInput => ({
  job: goJob,
  criteria,
  score: 0,
  hasScore: false,
  alreadyApplied: false,
  createdSoFar: 0,
  ...extra,
});

const validateCases: [string, Partial<Scout>, string][] = [
  ['a usable scout', {}, ''],
  ['no name', { name: '   ' }, 'needs a name'],
  ['no saved search', { savedSearchId: '' }, 'needs a saved search'],
  ['unknown source', { source: 'browser' }, 'unknown scout source'],
  ['negative score floor', { minMatchScore: -1 }, 'between 0 and 100'],
  ['impossible score floor', { minMatchScore: 101 }, 'between 0 and 100'],
  ['a run that can create nothing', { maxApplicationsPerRun: 0 }, 'between 1 and 50'],
  ['a run with no real ceiling', { maxApplicationsPerRun: 5000 }, 'between 1 and 50'],
  [
    'bad interval',
    { schedule: { kind: 'every', everyHours: 25, dailyAt: 0 } },
    'every 1 to 24 hours',
  ],
  [
    'bad daily time',
    { schedule: { kind: 'daily', everyHours: 0, dailyAt: 1440 } },
    'between 00:00 and 23:59',
  ],
  [
    'unknown schedule',
    { schedule: { kind: 'cron' as never, everyHours: 0, dailyAt: 0 } },
    'unknown schedule',
  ],
];
test.each(validateCases)('validate: %s', (_n, adjust, want) => {
  const err = validateScout(scout(adjust));
  if (want === '') expect(err).toBe('');
  else expect(err).toContain(want);
});

test('a matching job that clears the floor is eligible, and says why', () => {
  const [ok, reason] = eligible(scout(), input({ score: 85, hasScore: true }));
  expect(ok).toBe(true);
  expect(reason).toContain('85');
  expect(reason).toContain('70');
});
test('a job the search excludes is rejected before anything else', () => {
  const [ok, reason] = eligible(
    scout(),
    input({ job: job('Frontend Engineer'), score: 99, hasScore: true }),
  );
  expect(ok).toBe(false);
  expect(reason).toBe('does not match the saved search');
});
test('a job already applied to is rejected', () => {
  const [ok, reason] = eligible(
    scout(),
    input({ score: 99, hasScore: true, alreadyApplied: true }),
  );
  expect(ok).toBe(false);
  expect(reason).toContain('already exists');
});
test('a job that could not be scored is not treated as scoring zero', () => {
  const [ok, reason] = eligible(scout(), input());
  expect(ok).toBe(false);
  expect(reason).toContain('could not be scored');
});
test('a score below the floor is rejected and the numbers are named', () => {
  const [ok, reason] = eligible(scout(), input({ score: 69, hasScore: true }));
  expect(ok).toBe(false);
  expect(reason).toContain('69');
  expect(reason).toContain('70');
});
test('a score exactly on the floor clears it', () => {
  expect(eligible(scout(), input({ score: 70, hasScore: true }))[0]).toBe(true);
});
test('the ceiling stops a run that has made its quota', () => {
  const [ok, reason] = eligible(scout(), input({ score: 99, hasScore: true, createdSoFar: 5 }));
  expect(ok).toBe(false);
  expect(reason).toContain('limit of 5');
});
test('a floor of zero admits everything the search matched', () => {
  expect(eligible(scout({ minMatchScore: 0 }), input({ score: 0, hasScore: true }))[0]).toBe(true);
});
test('a missing job is refused rather than dereferenced', () => {
  const [ok, reason] = eligible(scout(), input({ job: null, score: 99, hasScore: true }));
  expect(ok).toBe(false);
  expect(reason).toContain('could not be read');
});
test('pre-screen skips the score', () => {
  expect(preScreen(scout(), input())[0]).toBe(true);
  expect(preScreen(scout(), input({ job: job('Frontend Engineer') }))).toEqual([
    false,
    'does not match the saved search',
  ]);
});
test('a job the user ruled out is passed over before anything else', () => {
  expect(
    preScreen(
      scout(),
      input({ job: job('Frontend Engineer', { notInterestedAt: '2026-09-25T10:00:00Z' }) }),
    ),
  ).toEqual([false, 'you marked this job not interested']);
});
test('scout sources', () => {
  expect(isValidSource('local')).toBe(true);
  expect(isValidSource('sources')).toBe(true);
  expect(isValidSource('')).toBe(false);
  expect(isValidSource('browser')).toBe(false);
});
