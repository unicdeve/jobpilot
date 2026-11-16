import { describe, expect, test } from 'bun:test';
import { checkDraft, draftBasisFor, letterWords, type Candidate, type DraftBasis, type DraftFlag, type DraftFlagKind } from './draft-check.ts';

// The cases are the Go test's (internal/core/domain/draftcheck_test.go), so the two cores cannot drift silently.
const NOW = new Date(Date.UTC(2025, 2, 1));

// A candidate with Go for four and a half years, Kubernetes with no duration, and a dated career of about six years.
const basisWith = (...ownWords: string[]): DraftBasis => {
  const candidate: Candidate = {
    summary: 'Backend engineer.',
    skills: [{ name: 'Go', yearsExperience: 4.5 }, { name: 'Kubernetes' }, { name: 'PostgreSQL', yearsExperience: 3 }],
    experiences: [{ startDate: '2019-03-01', isCurrent: true, description: '' }],
  };
  return draftBasisFor(candidate, NOW, ...ownWords);
};

const flagsOf = (kind: DraftFlagKind, flags: DraftFlag[]): DraftFlag[] => flags.filter((f) => f.kind === kind);

describe('stock phrases', () => {
  const cases: [string, string][] = [
    ['I am excited to apply for the Backend Engineer role.', 'I am excited to apply'],
    ["I'm thrilled to apply for this role.", "I'm thrilled to apply"],
    ['i AM EXCITED TO APPLY.', 'i AM EXCITED TO APPLY'],
    ['I believe I would be a great fit for your team.', 'a great fit'],
    ['Thank you for your consideration.', 'Thank you for your consideration'],
    ['I am writing to express my interest in the role.', 'I am writing to express my interest'],
    ['I’d welcome the opportunity to talk.', 'I’d welcome the opportunity'],
    ['I look forward to hearing from you.', 'I look forward to hearing from you'],
    ['I would be a valuable addition to\nyour platform team.', 'a valuable addition to'],
  ];
  for (const [text, want] of cases) {
    test(text, () => {
      const got = flagsOf('stock_phrase', checkDraft(text, basisWith()));
      expect(got).toHaveLength(1);
      expect(got[0]!.excerpt).toBe(want);
    });
  }

  test('plain writing is not flagged', () => {
    const text = "I rebuilt Acme's billing pipeline in Go, which cut failed charges by a third. Your post about idempotent retries is why I want this job.";
    expect(checkDraft(text, basisWith())).toEqual([]);
  });

  test('\\s is ASCII only, as in Go: a non-breaking space does not join the words', () => {
    expect(flagsOf('stock_phrase', checkDraft('a great fit', basisWith()))).toHaveLength(0);
    expect(flagsOf('stock_phrase', checkDraft('a   great\tfit', basisWith()))).toHaveLength(1);
  });

  test('\\b is ASCII: a non-ASCII letter next to a phrase is a boundary', () => {
    expect(flagsOf('stock_phrase', checkDraft('é team player é', basisWith()))).toHaveLength(1);
  });
});

describe('length', () => {
  const words = (n: number): string => 'word '.repeat(n).trim();

  test('flags a draft over the limit, once', () => {
    const basis = basisWith();
    basis.wordLimit = letterWords;
    expect(checkDraft(words(letterWords), basis)).toEqual([]);
    const got = flagsOf('length', checkDraft(words(300), basis));
    expect(got).toHaveLength(1);
    expect(got[0]!.reason).toBe('300 words, over the 250 a letter reads best within.');
    expect(got[0]!.excerpt).toBe('');
    basis.wordLimit = 0;
    expect(checkDraft(words(1000), basis)).toEqual([]);
  });

  test('counts words as strings.Fields does', () => {
    const basis = basisWith();
    basis.wordLimit = 2;
    expect(checkDraft('a b c', basis)).toHaveLength(1);
    expect(checkDraft('a b', basis)).toEqual([]);
  });
});

describe('years claims', () => {
  const tests: { name: string; text: string; own?: string[]; excerpt?: string; reason?: string }[] = [
    { name: "within a skill's duration, rounded", text: 'I have five years of Go experience.' },
    { name: "over a skill's duration", text: 'I bring 8+ years of Go.', excerpt: '8+ years', reason: 'Claims 8 years of Go; you gave 4.5 years.' },
    { name: 'a qualifier is kept', text: 'I have over six years with PostgreSQL.', excerpt: 'over six years', reason: 'Claims 6 years of PostgreSQL; you gave 3 years.' },
    { name: 'a skill with no duration', text: 'I have run Kubernetes for 3 years.', excerpt: '3 years', reason: 'Claims 3 years of Kubernetes, and you have not said how long you have used it.' },
    { name: 'a skill with no duration, in their own words', text: 'I have run Kubernetes for 3 years.', own: ['Ran Kubernetes in production for 3 years at Acme.'] },
    { name: 'their own words about another skill do not count', text: 'I have run Kubernetes for 3 years.', own: ['3 years of Terraform experience.'], excerpt: '3 years', reason: 'Claims 3 years of Kubernetes, and you have not said how long you have used it.' },
    { name: 'within the career', text: 'I have six years of experience building payment systems.' },
    { name: 'over the career', text: 'My ten years of experience in fintech taught me to test.', excerpt: 'ten years', reason: 'Claims 10 years; your work history covers about 6 years.' },
    { name: 'over the career, in their own words', text: 'I bring 10 years of experience.', own: ['10+ years of experience in backend engineering.'] },
    { name: 'a time ago', text: 'I moved to Lagos two years ago.' },
    { name: 'a time to come', text: 'I want to grow into a staff role in the next five years.' },
    { name: 'an age', text: 'I admire how your ten-year-old platform still ships weekly.' },
    { name: 'not about the candidate', text: 'Acme has been profitable for 12 years.' },
    { name: 'the skill Go is not the verb go', text: 'I have spent 9 years helping teams go faster.', excerpt: '9 years', reason: 'Claims 9 years; your work history covers about 6 years.' },
    // Beyond the Go cases: the regexp edge cases.
    { name: 'a decimal number of years', text: 'I have 7.5 years of Go.', excerpt: '7.5 years', reason: 'Claims 7.5 years of Go; you gave 4.5 years.' },
    { name: 'five-plus years', text: 'I have five-plus years of Go.', excerpt: 'five-plus years', reason: undefined },
    { name: 'seventeen is not seven', text: 'I have seventeen years of experience.', excerpt: 'seventeen years', reason: 'Claims 17 years; your work history covers about 6 years.' },
    { name: 'a single year', text: 'I have 1 year of Kubernetes.', excerpt: '1 year', reason: 'Claims 1 year of Kubernetes, and you have not said how long you have used it.' },
  ];
  for (const tt of tests) {
    test(tt.name, () => {
      const got = flagsOf('unsupported_claim', checkDraft(tt.text, basisWith(...(tt.own ?? []))));
      if (tt.excerpt === undefined) {
        expect(got).toEqual([]);
        return;
      }
      if (tt.name === 'five-plus years') {
        expect(got).toEqual([]); // 5 <= round(4.5)
        return;
      }
      expect(got).toHaveLength(1);
      expect(got[0]!.excerpt).toBe(tt.excerpt);
      expect(got[0]!.reason).toBe(tt.reason!);
    });
  }

  test('a skill name is matched without regexp injection', () => {
    const basis = draftBasisFor({ summary: '', skills: [{ name: 'C++', yearsExperience: 2 }, { name: 'C#' }], experiences: [] }, NOW);
    const got = flagsOf('unsupported_claim', checkDraft('I have 5 years of C++.', basis));
    expect(got).toHaveLength(1);
    expect(got[0]!.reason).toBe('Claims 5 years of C++; you gave 2 years.');
  });
});

test('no dated work history at all', () => {
  const basis = draftBasisFor({ summary: '', skills: [], experiences: [] }, new Date());
  const got = flagsOf('unsupported_claim', checkDraft('I have 3 years of experience.', basis));
  expect(got).toHaveLength(1);
  expect(got[0]!.reason).toBe('Claims 3 years, and your work history has no dates to check it against.');
});

test('the career is a span, not a sum', () => {
  const basis = draftBasisFor({
    summary: '', skills: [],
    experiences: [
      { startDate: '2019-03-01', endDate: '2021-03-01', isCurrent: false, description: '' },
      { startDate: '2020-03-01', endDate: '2025-03-01', isCurrent: false, description: '' },
      { isCurrent: false, description: '' },
    ],
  }, NOW);
  expect(basis.hasCareer).toBe(true);
  expect(Math.round(basis.careerYears)).toBe(6);
});

// The measured letter that prompted the check: every problem is flagged once.
test('the measured letter', () => {
  const letter = 'Dear Hiring Manager,\n\n'
    + 'I am excited to apply for the AI Engineer position. With 8 years of experience building agents, I believe I would be a great fit.\n\n'
    + 'I am excited to apply because your mission inspires me.\n\n'
    + 'Thank you for your consideration.';
  const flags = checkDraft(letter, basisWith());
  expect(flagsOf('stock_phrase', flags)).toHaveLength(3);
  expect(flagsOf('unsupported_claim', flags)).toHaveLength(1);
});
