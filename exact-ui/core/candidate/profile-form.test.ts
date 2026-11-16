import { expect, test } from 'bun:test';
import { emptyCandidate } from './candidate.ts';
import { ensureCandidateSchema, firstCandidate, saveCandidate } from './candidate-store.ts';
import { describeProfile, emptyProfileForm, parseAuthorizations, parseExperiences, parseLinks, parseSkills, profileFromForm, profileToForm, seekerOf } from './profile-form.ts';
import { memoryDb } from '../shared/test-db.ts';

const form = { ...emptyProfileForm, firstName: 'Ada', lastName: 'Lovelace', email: 'ada@example.com', timeZone: 'Africa/Lagos',
  links: 'github https://github.com/ada', authorizations: 'Nigeria: authorized\nGermany: needs sponsorship',
  skills: 'Go: 6, SQLite\nTypeScript', experiences: 'Engineer | Acme | 2022-07 | | Payments\nDev | Old Co | 2019-01 | 2022-06' };

test('a form is read into a candidate and written back unchanged', async () => {
  const read = profileFromForm(form, emptyCandidate('me', 0));
  expect(read.ok).toBe(true);
  if (!read.ok) return;
  expect(read.value.skills.map((s) => [s.name, s.yearsExperience])).toEqual([['Go', 6], ['SQLite', null], ['TypeScript', null]]);
  expect(read.value.experiences[0]).toMatchObject({ title: 'Engineer', company: 'Acme', isCurrent: true, endDate: null, description: 'Payments' });
  const db = memoryDb();
  await ensureCandidateSchema(db);
  const saved = await saveCandidate(db, read.value, 5, (() => { let n = 0; return () => `i${++n}`; })());
  expect(saved.ok).toBe(true);
  const again = profileToForm(await firstCandidate(db));
  expect(again.experiences).toBe('Engineer | Acme | 2022-07-01 | present | Payments\nDev | Old Co | 2019-01-01 | 2022-06-01');
  expect(profileFromForm(again, emptyCandidate('me', 0)).ok).toBe(true);
  expect(again.skills).toBe('Go: 6\nSQLite\nTypeScript');
  expect(describeProfile((await firstCandidate(db))!).authorizations).toEqual(['Nigeria: authorized', 'Germany: needs sponsorship']);
});

test('lines that cannot be read are errors naming the line, never guesses', () => {
  expect(parseLinks('myspace https://x.com')).toMatchObject({ ok: false });
  expect(parseLinks('github nonsense')).toMatchObject({ ok: false });
  expect(parseAuthorizations('Nigeria yes')).toMatchObject({ ok: false });
  expect(parseSkills('Go: lots')).toMatchObject({ ok: false });
  const e = parseExperiences('Dev | Acme | soon');
  expect(e.ok).toBe(false);
  expect(profileFromForm({ ...form, email: 'nope' }, emptyCandidate('me', 0))).toMatchObject({ ok: false });
});

test('a role with no dates is not made current', () => {
  const e = parseExperiences('Dev | Acme');
  expect(e.ok && e.value[0]!.isCurrent).toBe(false);
});

test('the seeker carries authorization and time zone only when given', () => {
  expect(seekerOf(undefined).hasTimeZone).toBe(false);
  const c = profileFromForm(form, emptyCandidate('me', 0));
  if (!c.ok) throw new Error(c.error);
  const s = seekerOf(c.value);
  expect(s.hasTimeZone).toBe(true);
  expect(s.authorizedCountries).toEqual(['NG']);
});
