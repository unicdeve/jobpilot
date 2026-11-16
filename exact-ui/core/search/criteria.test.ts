import { expect, test } from 'bun:test';
import {
  emptyCriteria,
  isEmpty,
  matches,
  matchesFor,
  mayMatch,
  noSeeker,
  searchTerms,
  type Job,
  type Seeker,
} from './criteria.ts';
import { gunzipSync } from 'node:zlib';
import { readFileSync } from 'node:fs';
import { loadCities } from './geo.ts';
import { readOffers } from './offers.ts';
import { sentences } from './signals.ts';
import { readWorkable } from './workable.ts';

const job: Job = {
  title: 'Engineer, Backend (Go)',
  companyName: 'Acme',
  descriptionMarkdown: 'Build services.',
  locations: ['Berlin'],
  remoteType: 'remote',
  employmentType: 'full_time',
  salaryMin: 0,
  salaryMax: 0,
};
const c = (over: Partial<typeof emptyCriteria>) => ({ ...emptyCriteria, ...over });

test('terms are matched in any order, as the Go rule does', () => {
  expect(searchTerms('  Backend  Engineer ')).toEqual(['backend', 'engineer']);
  expect(matches(c({ title: 'backend engineer' }), job)).toBe(true);
  expect(matches(c({ title: 'frontend' }), job)).toBe(false);
});
test('keywords read the description too', () => {
  expect(matches(c({ keywords: 'services' }), job)).toBe(true);
  expect(matches(c({ keywords: 'kubernetes' }), job)).toBe(false);
});
test('an unset criterion excludes nothing; a set one compares case-insensitively', () => {
  expect(matches(emptyCriteria, job)).toBe(true);
  expect(matches(c({ remotePreference: 'REMOTE' }), job)).toBe(true);
  expect(matches(c({ remotePreference: 'onsite' }), job)).toBe(false);
});
test('a job with no salary survives a salary bound, one with a salary is judged', () => {
  expect(matches(c({ salaryMin: 100000 }), job)).toBe(true);
  expect(matches(c({ salaryMin: 100000 }), { ...job, salaryMin: 50000, salaryMax: 60000 })).toBe(
    false,
  );
  expect(matches(c({ salaryMin: 55000 }), { ...job, salaryMin: 50000, salaryMax: 60000 })).toBe(
    true,
  );
  expect(matches(c({ salaryMax: 40000 }), { ...job, salaryMin: 50000, salaryMax: 60000 })).toBe(
    false,
  );
});
test('an excluded company is left out, ignoring case and spaces', () => {
  expect(matches(c({ excludedCompanies: [' acme '] }), job)).toBe(false);
});

// The signals a job carries into the rule, as ReadSignals sets them in Go (hours are given directly below).
loadCities(
  gunzipSync(
    readFileSync(new URL('../../../internal/core/domain/geo/cities.tsv.gz', import.meta.url)),
  ).toString('utf8'),
);
const readJob = (remoteType: string, locations: string[], description: string): Job => ({
  ...job,
  remoteType,
  locations,
  descriptionMarkdown: description,
  workable: readWorkable({ title: job.title, remoteType, locations }, sentences(description)),
  ...readOffers(description),
});

// These mirror TestSearchCriteria_Matches_WorkFrom in internal/core/domain/search_test.go.
test('work-from keeps jobs open to the candidate, or unsaid, and relocation brings in jobs elsewhere', () => {
  const anywhere = readJob('remote', ['Worldwide'], '');
  const emea = readJob('remote', ['EMEA'], '');
  const usOnly = readJob('remote', ['Remote (US)'], '');
  const unstated = readJob('remote', ['Remote'], '');
  const berlinRelocation = readJob(
    'onsite',
    ['Berlin'],
    'We offer an attractive relocation package.',
  );
  const berlinNoVisa = readJob(
    'onsite',
    ['Berlin'],
    'We offer relocation support. We are unable to offer visa sponsorship for this role.',
  );
  const berlinPlain = readJob('onsite', ['Berlin'], 'Join our team.');
  const unread: Job = { ...job, remoteType: 'remote', locations: ['Remote (US)'] };

  const fromNigeria = c({ workFrom: 'NG' });
  expect(matches(fromNigeria, anywhere)).toBe(true);
  expect(matches(fromNigeria, emea)).toBe(true);
  expect(matches(fromNigeria, usOnly)).toBe(false);
  expect(matches(fromNigeria, unstated)).toBe(true);
  expect(matches(fromNigeria, unread)).toBe(true);
  expect(matches(fromNigeria, berlinRelocation)).toBe(false);

  const moving = c({ workFrom: 'ng', includeRelocation: true });
  expect(matches(moving, berlinRelocation)).toBe(true);
  expect(matches(moving, berlinPlain)).toBe(false);
  expect(matches(moving, berlinNoVisa)).toBe(false);
  expect(matchesFor(moving, berlinNoVisa, { ...noSeeker, authorizedCountries: ['DE'] })).toBe(true);
  expect(matches(moving, usOnly)).toBe(false);
  expect(matches(c({}), usOnly)).toBe(true);
});

// These mirror TestSearchCriteria_FitHours.
test('fit-hours leaves out only a requirement the candidate cannot reach', () => {
  const lagos: Seeker = { authorizedCountries: [], utcOffset: 1, hasTimeZone: true };
  const eastern: Job = { ...job, hours: { status: 'required', from: -9, to: -1 } };
  const cet: Job = { ...job, hours: { status: 'required', from: -2, to: 4 } };
  const preferred: Job = { ...job, hours: { status: 'preferred', from: -8, to: -8 } };
  const strict = c({ title: 'engineer', fitHours: true });
  expect(matchesFor(strict, cet, lagos)).toBe(true);
  expect(matchesFor(strict, eastern, lagos)).toBe(false);
  expect(matchesFor(strict, preferred, lagos)).toBe(true);
  expect(matchesFor(strict, job, lagos)).toBe(true);
  expect(matchesFor(strict, eastern, noSeeker)).toBe(true);
  expect(matchesFor(c({ title: 'engineer', fitHours: true, hoursShift: 2 }), eastern, lagos)).toBe(
    true,
  );
  expect(matchesFor(c({ title: 'engineer' }), eastern, lagos)).toBe(true);
});

test('isEmpty ignores fitHours and relocation; mayMatch ignores what a description decides', () => {
  expect(isEmpty(emptyCriteria)).toBe(true);
  expect(isEmpty(c({ fitHours: true, includeRelocation: true }))).toBe(true);
  expect(isEmpty(c({ workFrom: 'NG' }))).toBe(false);
  const listed: Job = {
    ...job,
    title: 'Accountant',
    locations: ['Verl, NRW, Germany'],
    remoteType: 'onsite',
  };
  expect(mayMatch(c({ keywords: 'golang' }), listed)).toBe(true);
  expect(mayMatch(c({ workFrom: 'NG' }), listed)).toBe(true);
  expect(mayMatch(c({ title: 'engineer' }), listed)).toBe(false);
});

import { foldAscii, foldUnicode } from './geo.ts';
{
  test('fold: the ASCII path reads exactly as the Unicode rule does', () => {
    const samples = [
      'Bosnia and Herzegovina',
      'U.A.E.',
      "Cote d'Ivoire",
      '  Remote -- (EU) ',
      'São Paulo',
      'New York, NY',
      'a.b.c',
      '',
      'ABC-123_x/y',
    ];
    for (let i = 0; i < 500; i++)
      samples.push(
        Array.from({ length: i % 30 }, () =>
          String.fromCharCode(Math.floor(Math.random() * 128)),
        ).join(''),
      );
    for (const s of samples.filter((x) => /^[\x00-\x7f]*$/.test(x)))
      expect(foldAscii(s)).toBe(foldUnicode(s));
  });
}
