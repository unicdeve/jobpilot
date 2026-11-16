import { beforeAll, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { gunzipSync } from 'node:zlib';
import { loadCities } from './geo.ts';
import { readOffer, relocationTopic, visaTopic } from './offers.ts';
import { evidence, fields, maxEvidence, readSignals, sentences, signalsVersion, trimSpace, unescapeHtml } from './signals.ts';
import { openTo, workableKnown } from './workable.ts';

beforeAll(() => {
  loadCities(gunzipSync(readFileSync(new URL('../../../internal/core/domain/geo/cities.tsv.gz', import.meta.url))).toString('utf8'));
});

const sig = (remoteType: string, locations: string[], descriptionMarkdown = '', title = '') =>
  readSignals({ title, remoteType, locations, descriptionMarkdown });

// The first case is the Go test's (internal/core/domain/signals_test.go), so the two cores cannot drift silently.
test('sentences reads plain text out of html and markdown', () => {
  expect(sentences('&lt;p&gt;We are **unable** to sponsor U.S. visas.&lt;/p&gt;\n- Relocation support | [https://acme.example](https://acme.example/jobs)'))
    .toEqual(['We are unable to sponsor US visas', 'Relocation support', 'https://acme.example']);
});

test('sentences splits on lines, list marks, pipes and terminators', () => {
  expect(sentences('One. Two!  Three?\nFour\n\n* Five - Six | Seven')).toEqual(['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven']);
  expect(sentences('$250k+ base | Visa sponsorship available')).toEqual(['$250k+ base', 'Visa sponsorship available']);
  expect(sentences('')).toEqual([]);
  expect(sentences('> quoted\n- item')).toEqual(['quoted', 'item']);
});

test('sentences keeps abbreviations whole', () => {
  expect(sentences('Open to U.S.A. staff, e.g. Lagos.')).toEqual(['Open to USA staff, eg Lagos']);
  expect(sentences('U.K. and U.A.E. i.e. remote, approx. 5 incl. visas')).toEqual(['UK and UAE ie remote, approx 5 incl visas']);
});

test('sentences unescapes entities, also when escaped twice', () => {
  expect(sentences('Fish &amp;amp; chips')).toEqual(['Fish & chips']);
  expect(sentences('Caf&eacute; &#233; &#xE9; &mdash; ok')).toEqual(['Café é é — ok']);
  expect(unescapeHtml('&amp &unknown; &#0; &#x110000; &#150;')).toEqual('& &unknown; � � –');
});

test('sentences handles non-ASCII whitespace like Go: NBSP is not collapsed but is trimmed', () => {
  expect(sentences('a  b')).toEqual(['a  b']);
  expect(sentences('  hello 　')).toEqual(['hello']);
  expect(sentences('a b')).toEqual(['a b']);
});

test('strings.Fields and TrimSpace semantics', () => {
  expect(fields(' a b c\u0085d﻿e ')).toEqual(['a', 'b', 'c', 'd﻿e']);
  expect(trimSpace('\u0085 x ﻿')).toEqual('x ﻿');
});

test('evidence keeps a short sentence and cuts a long one at a word', () => {
  expect(evidence('short')).toBe('short');
  const long = `${'word '.repeat(100)}end`;
  const cut = evidence(long);
  expect(cut.endsWith('word…')).toBe(true);
  expect(cut.length).toBeLessThanOrEqual(maxEvidence + 1);
  expect(evidence('x'.repeat(400))).toBe(`${'x'.repeat(maxEvidence)}…`);
});

test('the signals version matches Go', () => {
  expect(signalsVersion).toBe(3);
});

const locationCases: [string, string, string[], string, string[] | null][] = [
  ['worldwide', 'remote', ["Worldwide"], 'anywhere', null],
  ['anywhere in the world', 'remote', ["Anywhere in the World"], 'anywhere', null],
  ['remote global', 'remote', ["Remote (Global)"], 'anywhere', null],
  ['remote everywhere', 'remote', ["Remote(Everywhere)"], 'anywhere', null],
  ['anywhere with hours', 'remote', ["REMOTE (anywhere, EST hours)"], 'anywhere', null],
  ['remote us', 'remote', ["Remote (US)"], 'listed', ["US"]],
  ['us fully remote', 'remote', ["US (Fully Remote)"], 'listed', ["US"]],
  ['remote dash us', 'remote', ["Remote - US"], 'listed', ["US"]],
  ['us citizens', 'remote', ["U.S. Citizens"], 'listed', ["US"]],
  ['us only with cities', 'remote', ["REMOTE (US ONLY, LA/SF preferred)"], 'listed', ["US"]],
  ['us time zones only', 'remote', ["USA, Canada, USA timezones"], 'listed', ["CA", "US"]],
  ['himalayas countries', 'remote', ["United States", "Poland"], 'listed', ["PL", "US"]],
  ['bare remote', 'remote', ["Remote"], 'unknown', null],
  ['no location', 'remote', [], 'unknown', null],
  ['time zone only', 'remote', ["REMOTE (US time zones)"], 'unknown', null],
  ['exclusion list', 'remote', ["Internationally located (not in the US, CA, UK, NZ, or AU)"], 'unknown', null],
  ['not a place', 'remote', ["Full-time*"], 'unknown', null],
  ['onsite city', 'onsite', ["München"], 'listed', ["DE"]],
  ['onsite city and country', 'onsite', ["London, England, United Kingdom"], 'listed', ["GB"]],
  ['onsite state', 'onsite', ["Hamburg, Hamburg, Deutschland"], 'listed', ["DE"]],
  ['onsite either city', 'hybrid', ["London, UK or NYC"], 'listed', ["GB", "US"]],
  ['onsite parenthesised', 'onsite', ["ONSITE (Zürich, Switzerland)"], 'listed', ["CH"]],
  ['hybrid with terms', 'hybrid', ["Hybrid (Belgium, 3 days/week)"], 'listed', ["BE"]],
  ['onsite anywhere is still its office', 'onsite', ["Berlin", "Worldwide"], 'listed', ["DE"]],
  ['onsite unplaced town', 'onsite', ["Grünwald"], 'unknown', null],
  ['city named by its state', 'onsite', ["On-Site (Cambridge, MA)"], 'listed', ["US"]],
  ['city named by its province', 'onsite', ["London, ON"], 'listed', ["CA"]],
  ['most populous city', 'onsite', ["Cambridge"], 'listed', ["GB"]],
  ['country code is not a state', 'onsite', ["Hamburg, DE"], 'listed', ["DE"]],
  ['city outside its country', 'onsite', ["Paris, Texas"], 'listed', ["US"]],
  ["a state that is a country's name, beside its country", 'remote', ["Remote, US, Georgia"], 'listed', ["US"]],
  ["a state that is a country's name, beside its city", 'onsite', ["Atlanta, Georgia"], 'listed', ["US"]],
  ["a state that is a country's name, by its code", 'remote', ["Georgia, GA"], 'listed', ["US"]],
  ['the country Georgia', 'onsite', ["Tbilisi, Georgia"], 'listed', ["GE"]],
  ['the country Georgia, alone', 'remote', ["Georgia"], 'listed', ["GE"]],
  ['the country Georgia, beside another country', 'remote', ["Georgia; Armenia"], 'listed', ["AM", "GE"]],
  ['compound country names', 'remote', ['🇧🇦 Bosnia and Herzegovina, 🇹🇼 Taiwan, Province of China, 🇰🇷 Korea (Republic of), and 🇬🇧 United Kingdom of Great Britain and Northern Ireland'], 'listed', ['BA', 'GB', 'KR', 'TW']],
  ['regions', 'remote', ["Northern America, LATAM, Europe, APAC"], 'listed', null],
];
test('readSignals reads where a job can be done from, as in Go', () => {
  for (const [name, remote, locations, scope, countries] of locationCases) {
    const w = sig(remote, locations).workable;
    expect({ name, scope: w.scope }).toEqual({ name, scope });
    if (countries !== null) expect({ name, countries: w.countries }).toEqual({ name, countries });
  }
});

test('regions expand', () => {
  const w = sig('remote', ['EMEA']).workable;
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
    const w = sig('remote', locations, description).workable;
    if (contradicts) {
      expect({ name, scope: w.scope }).toEqual({ name, scope: 'unknown' });
      expect(w.note).toBeTruthy();
    } else {
      expect({ name, known: workableKnown(w) }).toEqual({ name, known: true });
    }
  }
});

test('a title can contradict the label', () => {
  const titles: [string, boolean][] = [
    ['Staff Backend Engineer - Core Product (Europe)', false],
    ['Senior Backend Engineer, AI Platform - 100% Remote - EMEA', true],
    ['Backend Engineer - Wallets (100% Remote Worldwide)', true],
    ['Backend Software Engineer - Python/Postgres [Remote / Global]', true],
    ['Go - Senior Backend Engineer, AI Coding Agent Quality', true],
    ['Senior Engineer (US)', false],
    ['Senior Backend Engineer, Integrations - 100% Remote - EMEA', true],
  ];
  for (const [title, known] of titles) {
    // Himalayas' own list of EMEA differs from JobPilot's at its edges.
    const locations = title.includes('EMEA') ? ['Afghanistan', 'Albania', 'Germany', 'Nigeria', 'Kazakhstan'] : ['Worldwide'];
    const w = sig('remote', locations, '', title).workable;
    expect({ title, known: workableKnown(w) }).toEqual({ title, known });
    if (!known) expect(w.note).toBe(title);
  }
});

const status = (text: string, topic: typeof relocationTopic) => readOffer(sentences(text), topic).status;

test('relocation offers', () => {
  const cases: [string, string][] = [
    ['We welcome applications from talent worldwide and provide relocation support to Hamburg, Germany.', 'offered'],
    ['Full relocation to SF covered. Onsite in SF.', 'offered'],
    ['- Relocation support (up to €10k) for roles based in France', 'offered'],
    ['Do you want to move to Germany? No problem – we offer you an attractive relocation package.', 'offered'],
    ['We are open to support with relocation efforts.', 'offered'],
    ['We are currently unable to offer fully remote work, visa sponsorship, or relocation support for this opportunity.', 'declined'],
    ['Relocation assistance will not be provided for this role.', 'declined'],
    ['This position is not eligible for relocation assistance.', 'declined'],
    ['Please note: For this role we are not offering relocation support or visa sponsorship.', 'declined'],
    ['Visa sponsorship may be available for eligible candidates already located in a UK/EU country; relocation is not provided.', 'declined'],
    ['Based in or willing to relocate to San Francisco, CA or Austin, TX.', 'mentioned'],
    ['Relocation assistance (position dependent).', 'mentioned'],
    ['Benefits vary by country and may include healthcare coverage, parental leave and relocation support.', 'mentioned'],
    ['We are not a typical company, and we offer relocation support to everyone who joins.', 'offered'],
    ['Our platform processes 25 trillion data points.', ''],
  ];
  for (const [text, want] of cases) expect({ text, got: status(text, relocationTopic) }).toEqual({ text, got: want });
});

test('visa sponsorship offers', () => {
  const cases: [string, string][] = [
    ['$250k+ base + seed equity | Visa sponsorship available', 'offered'],
    ['Visa sponsorship: yes.', 'offered'],
    ['Why join us: €10k signing bonus - Visa sponsorship + relocation support', 'offered'],
    ['Applicants must be based in the US, we are unable to sponsor visas.', 'declined'],
    ['Software Engineering Intern | ONSITE (Zürich, Switzerland) | Internship | No visa sponsorship', 'declined'],
    ['The role touches ITAR-controlled data, so US persons only, no visa sponsorship.', 'declined'],
    ['Candidates must be authorized to work in the United States without current or future employer sponsorship.', 'declined'],
    ['Unfortunately, we do not offer sponsorship and you do need to be living in the UK or Ireland to apply.', 'declined'],
    ['We do not provide visa assistance, and our cooperation model does not include benefits.', 'declined'],
    ['$180-200K + equity | VISA (transfer only)', 'mentioned'],
    ['Proactively mentor, sponsor and develop senior engineers.', ''],
    ['Company-sponsored dinners and social gatherings.', ''],
    ['An employer-sponsored 401(k) retirement plan.', ''],
  ];
  for (const [text, want] of cases) expect({ text, got: status(text, visaTopic) }).toEqual({ text, got: want });
});

test('a refusal wins and keeps its sentence', () => {
  const offer = readOffer(sentences('Benefits may include relocation support.\n\nThis position is not eligible for relocation assistance.'), relocationTopic);
  expect(offer.status).toBe('declined');
  expect(offer.evidence).toBe('This position is not eligible for relocation assistance');
});

test('readSignals sets every signal and the version', () => {
  const s = sig('remote', ['Worldwide'], 'Visa sponsorship available. Relocation support offered.');
  expect(s.visaSponsorship.status).toBe('offered');
  expect(s.relocation.status).toBe('offered');
  expect(s.signalsVersion).toBe(signalsVersion);
});
