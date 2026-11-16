import { expect, test } from 'bun:test';
import { evidence, fold, offerAvailable, readOffer, readOffers, relocationTopic, sentences, visaTopic, type OfferStatus } from './offers.ts';

// The cases are the Go tests' (internal/core/domain/signals_test.go: TestReadOffer_*, TestSentences_PlainText).
const relocation: [string, OfferStatus][] = [
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
];
test.each(relocation)('relocation: %s', (text, want) => expect(readOffer(sentences(text), relocationTopic).status).toBe(want));

const visa: [string, OfferStatus][] = [
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
];
test.each(visa)('visa: %s', (text, want) => expect(readOffer(sentences(text), visaTopic).status).toBe(want));

test('a refusal wins and keeps its sentence', () => {
  const offer = readOffer(sentences('Benefits may include relocation support.\n\nThis position is not eligible for relocation assistance.'), relocationTopic);
  expect(offer).toEqual({ status: 'declined', evidence: 'This position is not eligible for relocation assistance' });
});

test('sentences are plain text', () => {
  expect(sentences('&lt;p&gt;We are **unable** to sponsor U.S. visas.&lt;/p&gt;\n- Relocation support | [https://acme.example](https://acme.example/jobs)'))
    .toEqual(['We are unable to sponsor US visas', 'Relocation support', 'https://acme.example']);
});

test('a posting that says nothing has no offer', () => {
  const o = readOffers('We build tools for plumbers. Company-sponsored dinners every month.');
  expect(o.relocation.status).toBe('');
  expect(o.visaSponsorship.status).toBe('');
  expect(offerAvailable(o.relocation)).toBe(false);
  expect(offerAvailable({ status: 'mentioned', evidence: '' })).toBe(true);
});

test('evidence is bounded at a word', () => {
  const long = `${'word '.repeat(80)}end`;
  const e = evidence(long);
  expect(e.endsWith('…')).toBe(true);
  expect(e.length).toBeLessThanOrEqual(281);
  expect(evidence('short')).toBe('short');
});

test('fold', () => {
  expect(fold('Zürich')).toBe('zurich');
  expect(fold("U.S. don't")).toBe('us don t');
});
