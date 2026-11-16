import { beforeAll, expect, test } from 'bun:test';
import { gunzipSync } from 'node:zlib';
import { readFileSync } from 'node:fs';
import { loadCities, lookup, resolveCountry, subdivisionOf, timeZoneByID } from './geo.ts';
import { readHours } from './hours.ts';
import { sentences } from './signals.ts';
import { openTo, readWorkable, workableKnown, type Workable } from './workable.ts';

// The cases are the Go tests' (internal/core/domain/signals_test.go, geo/geo_test.go). The city table is the Go package's own file;
// the app installs it the same way with loadCities.
beforeAll(() => {
  const gz = readFileSync(new URL('../../../internal/core/domain/geo/cities.tsv.gz', import.meta.url));
  loadCities(gunzipSync(gz).toString('utf8'));
});

const read = (remoteType: string, locations: string[], description = '', title = ''): Workable =>
  readWorkable({ title, remoteType, locations }, sentences(description));

const locationCases: [string, string, string[], string, string[] | null][] = [
  ['worldwide', 'remote', ['Worldwide'], 'anywhere', null],
  ['anywhere in the world', 'remote', ['Anywhere in the World'], 'anywhere', null],
  ['remote global', 'remote', ['Remote (Global)'], 'anywhere', null],
  ['remote everywhere', 'remote', ['Remote(Everywhere)'], 'anywhere', null],
  ['anywhere with hours', 'remote', ['REMOTE (anywhere, EST hours)'], 'anywhere', null],
  ['remote us', 'remote', ['Remote (US)'], 'listed', ['US']],
  ['us fully remote', 'remote', ['US (Fully Remote)'], 'listed', ['US']],
  ['remote dash us', 'remote', ['Remote - US'], 'listed', ['US']],
  ['us citizens', 'remote', ['U.S. Citizens'], 'listed', ['US']],
  ['us only with cities', 'remote', ['REMOTE (US ONLY, LA/SF preferred)'], 'listed', ['US']],
  ['country list', 'remote', ['USA, Canada, Argentina, Mexico, Peru'], 'listed', ['AR', 'CA', 'MX', 'PE', 'US']],
  ['us time zones only', 'remote', ['USA, Canada, USA timezones'], 'listed', ['CA', 'US']],
  ['himalayas countries', 'remote', ['United States', 'Poland'], 'listed', ['PL', 'US']],
  ['bare remote', 'remote', ['Remote'], 'unknown', null],
  ['no location', 'remote', [], 'unknown', null],
  ['time zone only', 'remote', ['REMOTE (US time zones)'], 'unknown', null],
  ['exclusion list', 'remote', ['Internationally located (not in the US, CA, UK, NZ, or AU)'], 'unknown', null],
  ['not a place', 'remote', ['Full-time*'], 'unknown', null],
  ['onsite city', 'onsite', ['München'], 'listed', ['DE']],
  ['onsite city and country', 'onsite', ['London, England, United Kingdom'], 'listed', ['GB']],
  ['onsite state', 'onsite', ['Hamburg, Hamburg, Deutschland'], 'listed', ['DE']],
  ['onsite either city', 'hybrid', ['London, UK or NYC'], 'listed', ['GB', 'US']],
  ['onsite parenthesised', 'onsite', ['ONSITE (Zürich, Switzerland)'], 'listed', ['CH']],
  ['hybrid with terms', 'hybrid', ['Hybrid (Belgium, 3 days/week)'], 'listed', ['BE']],
  ['onsite anywhere is still its office', 'onsite', ['Berlin', 'Worldwide'], 'listed', ['DE']],
  ['onsite unplaced town', 'onsite', ['Grünwald'], 'unknown', null],
  ['city named by its state', 'onsite', ['On-Site (Cambridge, MA)'], 'listed', ['US']],
  ['city named by its province', 'onsite', ['London, ON'], 'listed', ['CA']],
  ['most populous city', 'onsite', ['Cambridge'], 'listed', ['GB']],
  ['country code is not a state', 'onsite', ['Hamburg, DE'], 'listed', ['DE']],
  ['city outside its country', 'onsite', ['Paris, Texas'], 'listed', ['US']],
  ["a state that is a country's name, beside its country", 'remote', ['Remote, US, Georgia'], 'listed', ['US']],
  ["a state that is a country's name, beside its city", 'onsite', ['Atlanta, Georgia'], 'listed', ['US']],
  ["a state that is a country's name, by its code", 'remote', ['Georgia, GA'], 'listed', ['US']],
  ['the country Georgia', 'onsite', ['Tbilisi, Georgia'], 'listed', ['GE']],
  ['the country Georgia, alone', 'remote', ['Georgia'], 'listed', ['GE']],
  ['the country Georgia, beside another country', 'remote', ['Georgia; Armenia'], 'listed', ['AM', 'GE']],
  ['compound country names', 'remote',
    ['🇧🇦 Bosnia and Herzegovina, 🇹🇼 Taiwan, Province of China, 🇰🇷 Korea (Republic of), and 🇬🇧 United Kingdom of Great Britain and Northern Ireland'],
    'listed', ['BA', 'GB', 'KR', 'TW']],
  ['regions', 'remote', ['Northern America, LATAM, Europe, APAC'], 'listed', null],
];

test('readWorkable reads locations', () => {
  for (const [name, remote, locations, scope, countries] of locationCases) {
    const w = read(remote, locations);
    expect([name, w.scope]).toEqual([name, scope]);
    if (countries) expect([name, w.countries]).toEqual([name, countries]);
  }
});

test('a region expands to its countries', () => {
  const w = read('remote', ['EMEA']);
  expect(w.places).toEqual(['EMEA']);
  expect(openTo(w, 'NG')).toBe(true);
  expect(openTo(w, 'de')).toBe(true);
  expect(openTo(w, 'US')).toBe(false);
});

const contradictions: [string, string[], string, boolean][] = [
  ['must reside', ['Anywhere in the World'], 'Candidates must reside in or be willing to relocate to the San Francisco Bay Area (Alameda, Contra Costa).', true],
  ['legally within the us', ['Anywhere in the World'], 'Applicants must be able to work legally within the United States. Please, no 3rd party inquiries.', false],
  ['authorized in the us', ['Anywhere in the World'], "We're only hiring those authorized to work in the United States.", true],
  ['us based only', ['Anywhere in the World'], 'US based applicants only. $114,297 — $235,319 USD', true],
  ['located in several', ['Worldwide'], 'This is a remote position. However, candidates must be located in the United States, Canada (Ontario or British Columbia), Netherlands, Poland.', true],
  ['consistent with the label', ['Remote (US)'], 'Candidates must be authorized to work in the United States without sponsorship.', false],
  ['narrower than the label', ['Europe'], 'You must be based in Germany to be considered.', false],
  ['elsewhere than the label', ['Europe'], 'You must be authorized to work in the United States.', true],
  ['time zone', ['Worldwide'], 'You must be located within three hours of CET time zone.', false],
  ['no place named', ['Worldwide'], 'You must be eligible to work in the country you live in.', false],
  ['us the pronoun', ['Worldwide'], 'Remote from anywhere; let us know where you are based and we will work with you.', false],
];

test('a description can contradict the label', () => {
  for (const [name, locations, description, contradicts] of contradictions) {
    const w = read('remote', locations, description);
    if (contradicts) {
      expect([name, w.scope]).toEqual([name, 'unknown']);
      expect(w.note).toBeTruthy();
    } else {
      expect([name, workableKnown(w)]).toEqual([name, true]);
    }
  }
});

test('a title can contradict the label', () => {
  const titles: Record<string, boolean> = {
    'Staff Backend Engineer - Core Product (Europe)': false,
    'Senior Backend Engineer, AI Platform - 100% Remote - EMEA': true,
    'Backend Engineer - Wallets (100% Remote Worldwide)': true,
    'Backend Software Engineer - Python/Postgres [Remote / Global]': true,
    'Go - Senior Backend Engineer, AI Coding Agent Quality': true,
    'Senior Engineer (US)': false,
    'Senior Backend Engineer, Integrations - 100% Remote - EMEA': true,
  };
  for (const [title, known] of Object.entries(titles)) {
    // Himalayas' own list of EMEA, which differs from JobPilot's at its edges.
    const locations = title.includes('EMEA') ? ['Afghanistan', 'Albania', 'Germany', 'Nigeria', 'Kazakhstan'] : ['Worldwide'];
    const w = read('remote', locations, '', title);
    expect([title, workableKnown(w)]).toEqual([title, known]);
    if (!known) expect(w.note).toBe(title);
  }
});

test('where and when stay separate', () => {
  const description = 'We build things.\n\n- Located in CET timezone (+/- 3 hours).\n- Python';
  expect(read('remote', ['Anywhere in the World'], description).scope).toBe('anywhere');
  const hours = readHours(sentences(description));
  expect([hours.status, hours.from, hours.to]).toEqual(['required', -2, 4]);
});

test('geo lookups', () => {
  expect(lookup('Georgia', false)?.countries).toEqual(['GE']);
  expect(lookup('Northern America', false)?.countries).toEqual(['CA', 'MX', 'US']);
  expect(lookup('London', false)).toBeUndefined();
  expect(lookup('NYC', true)?.name).toBe('New York City');
  expect(lookup('Tor', true)).toBeUndefined();
  expect(subdivisionOf('Georgia')).toBe('US');
  expect(subdivisionOf('Texas')).toBeUndefined();
  expect(resolveCountry(' Deutschland ')).toBe('DE');
  expect(resolveCountry('EMEA')).toBeUndefined();
  expect(timeZoneByID('asia/kolkata')?.offset).toBe(5.5);
  expect(timeZoneByID('Africa/Lagos')?.offset).toBe(1);
  expect(timeZoneByID('Mars/Olympus')).toBeUndefined();
});
