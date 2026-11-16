// A port of the Go core's draft checks (internal/core/domain/draftcheck.go), ADR-0037.
// It has no Exact, network or storage dependency, and reads no clock (`now` is a parameter), so it runs the same in the app's JS engine and under `bun test`.
// Go's regexp is RE2: \s is ASCII only, so it is spelled out here; \b and \w are ASCII in both engines.
import { fields, sentences, trimSpace } from '../search/signals.ts';

export type DraftFlagKind = 'stock_phrase' | 'length' | 'unsupported_claim';

export interface DraftFlag {
  kind: DraftFlagKind;
  // The flagged words as the draft has them; a length flag has none.
  excerpt: string;
  reason: string;
}

export const letterWords = 250;

export interface CandidateSkill {
  name: string;
  yearsExperience?: number | null;
}

// A calendar date, YYYY-MM-DD (or YYYY-MM, or YYYY, as the Go Date reads them).
export interface CandidateExperience {
  startDate?: string | null;
  endDate?: string | null;
  isCurrent: boolean;
  description: string;
}

export interface Candidate {
  summary: string;
  skills: CandidateSkill[];
  experiences: CandidateExperience[];
}

export interface DraftBasis {
  // The most words the draft may have; zero checks no length.
  wordLimit: number;
  skills: CandidateSkill[];
  careerYears: number;
  hasCareer: boolean;
  ownWords: string[];
}

const DAY_MS = 86_400_000;

const parseDate = (s: string): number | null => {
  const m = /^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?/.exec(s.trim());
  if (!m) return null;
  return Date.UTC(Number(m[1]), m[2] ? Number(m[2]) - 1 : 0, m[3] ? Number(m[3]) : 1);
};

// Candidate.TotalYearsOfExperience: the span from the earliest start to the latest end (or now), never a sum.
export function totalYearsOfExperience(candidate: Candidate, now: Date): { years: number; ok: boolean } {
  let earliest: number | null = null;
  let latest: number | null = null;
  for (const e of candidate.experiences) {
    const start = e.startDate ? parseDate(e.startDate) : null;
    if (start === null) continue;
    if (earliest === null || start < earliest) earliest = start;
    let end = now.getTime();
    if (!e.isCurrent && e.endDate) {
      const parsed = parseDate(e.endDate);
      if (parsed !== null) end = parsed;
    }
    if (latest === null || end > latest) latest = end;
  }
  if (earliest === null || latest === null || !(latest > earliest)) return { years: 0, ok: false };
  return { years: (latest - earliest) / (DAY_MS * 365.25), ok: true };
}

// Gathers a candidate's facts into a basis; it sets no word limit (a letter sets letterWords).
export function draftBasisFor(candidate: Candidate, now: Date, ...ownWords: string[]): DraftBasis {
  const career = totalYearsOfExperience(candidate, now);
  return {
    wordLimit: 0,
    skills: candidate.skills,
    careerYears: career.years,
    hasCareer: career.ok,
    ownWords: [candidate.summary, ...candidate.experiences.map((e) => e.description), ...ownWords],
  };
}

const WS = '[\\t\\n\\f\\r ]';
// Placeholders, so the literal-space rewrite below does not touch the whitespace class inside them.
const iAm = '§AM§';
const iWould = '§WOULD§';

// The openings, closings and fillers that make a letter read as anyone's.
// Written with literal spaces, which become \s+ as in the Go source.
const stockSources = [
  `${iAm} (?:so |very |truly )?(?:excited|thrilled|delighted|eager) to apply`,
  `${iAm} (?:so |very |truly )?(?:excited|thrilled|delighted) (?:about|by) the opportunity`,
  `${iAm} writing to (?:express my (?:strong |keen )?interest|apply)`,
  '(?:a|an|the) (?:great|perfect|ideal|strong|excellent) fit',
  'thank you for (?:your (?:time and )?consideration|considering my application|your time)',
  'i look forward to hearing from you',
  `${iWould} welcome the (?:opportunity|chance)`,
  `${iAm} confident (?:that )?i`,
  'hit the ground running',
  'team player',
  'proven track record',
  'to whom it may concern',
  '(?:a )?valuable (?:asset|addition) to',
  'unique blend of',
  'aligns? perfectly with',
  `${iAm} passionate about`,
];
const stockPhrases = stockSources.map((s) => new RegExp(`\\b${s.replace(/ /g, `${WS}+`).replace('§AM§', `i(?:${WS}+am|['’]m)`).replace('§WOULD§', `i(?:${WS}+would|['’]d)`)}\\b`, 'gi'));

interface Claim {
  years: number;
  excerpt: string;
  skills: CandidateSkill[];
}

const yearWords: Record<string, number> = {
  one: 1, two: 2, three: 3, four: 4, five: 5, six: 6, seven: 7, eight: 8, nine: 9, ten: 10, eleven: 11, twelve: 12, thirteen: 13,
  fourteen: 14, fifteen: 15, sixteen: 16, seventeen: 17, eighteen: 18, nineteen: 19, twenty: 20,
};

// "8 years", "8+ years", "over eight years", "five-plus years", "a 3-year stint". Group 3 makes it an age or a time ago.
const yearsSource = '\\b((?:over|more than|nearly|almost|about|around)' + WS + '+)?'
  + '(\\d{1,2}(?:\\.\\d+)?|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty)'
  + `(?:${WS}*\\+|-plus|${WS}+plus)?[${'\\t\\n\\f\\r '}-]+years?\\b(-old|${WS}+old|${WS}+ago)?`;

// A sentence that speaks for the candidate or of experience.
const claimSentence = /\b(?:i|i['’](?:m|ve|d)|my|me|experience[sd]?)\b/i;

const futureWords = new Set(['next', 'coming', 'within']);

const claimsIn = (texts: string[], skills: SkillMatcher[]): Claim[] => {
  const claims: Claim[] = [];
  for (const text of texts) for (const sentence of sentences(text)) claims.push(...yearsIn(sentence, skills));
  return claims;
};

const sentenceClaims = (sentence: string, skills: SkillMatcher[]): Claim[] =>
  claimSentence.test(sentence) ? yearsIn(sentence, skills) : [];

// Every number of years in a sentence that is neither an age, a time ago nor a time to come.
function yearsIn(sentence: string, skills: SkillMatcher[]): Claim[] {
  const claims: Claim[] = [];
  const re = new RegExp(yearsSource, 'gi');
  for (let m = re.exec(sentence); m !== null; m = re.exec(sentence)) {
    if (m[3] !== undefined) continue; // an age, or a time ago
    const words = fields(sentence.slice(0, m.index));
    const last = words[words.length - 1];
    if (last !== undefined && futureWords.has(last.toLowerCase())) continue;
    const number = m[2]!.toLowerCase();
    const years = yearWords[number] ?? Number.parseFloat(number);
    if (Number.isNaN(years)) continue;
    claims.push({ years, excerpt: m[0], skills: skillsNamed(sentence, skills) });
  }
  return claims;
}

interface SkillMatcher {
  skill: CandidateSkill;
  pattern: RegExp;
}

const quoteMeta = (s: string): string => s.replace(/[\\.+*?()|[\]{}^$]/g, '\\$&');

// A name of three letters or fewer must match its case, so the skill Go is not found in "helping teams go faster".
function skillMatchers(skills: CandidateSkill[]): SkillMatcher[] {
  const matchers: SkillMatcher[] = [];
  for (const skill of skills) {
    const name = trimSpace(skill.name);
    if (name === '') continue;
    const pattern = `(?:^|[^\\w+#.])${quoteMeta(name)}(?:$|[^\\w+#])`;
    matchers.push({ skill, pattern: new RegExp(pattern, [...name].length > 3 ? 'i' : '') });
  }
  return matchers;
}

const skillsNamed = (sentence: string, skills: SkillMatcher[]): CandidateSkill[] =>
  skills.filter((m) => m.pattern.test(sentence)).map((m) => m.skill);

// Go's math.Round: half away from zero.
const goRound = (n: number): number => (n < 0 ? -Math.round(-n) : Math.round(n));

export const yearsText = (years: number): string => `${years} ${years === 1 ? 'year' : 'years'}`;

function unsupported(c: Claim, basis: DraftBasis, own: Claim[]): string | null {
  if (saidThemselves(c, own)) return null;
  if (c.skills.length > 0) {
    for (const skill of c.skills) {
      if (skill.yearsExperience === undefined || skill.yearsExperience === null) {
        return `Claims ${yearsText(c.years)} of ${skill.name}, and you have not said how long you have used it.`;
      }
      if (c.years > goRound(skill.yearsExperience)) {
        return `Claims ${yearsText(c.years)} of ${skill.name}; you gave ${yearsText(skill.yearsExperience)}.`;
      }
    }
    return null;
  }
  if (!basis.hasCareer) return `Claims ${yearsText(c.years)}, and your work history has no dates to check it against.`;
  if (c.years > goRound(basis.careerYears)) {
    return `Claims ${yearsText(c.years)}; your work history covers about ${yearsText(goRound(basis.careerYears))}.`;
  }
  return null;
}

// The candidate's own words claim at least as many years about every skill the claim names.
const saidThemselves = (c: Claim, own: Claim[]): boolean =>
  own.some((theirs) => theirs.years >= c.years && c.skills.every((s) => theirs.skills.some((t) => t.name === s.name)));

// What the candidate may want to look at in a letter or written answer; the same text and basis always give the same flags.
export function checkDraft(text: string, basis: DraftBasis): DraftFlag[] {
  const flags: DraftFlag[] = [];
  if (basis.wordLimit > 0) {
    const words = fields(text).length;
    if (words > basis.wordLimit) {
      flags.push({ kind: 'length', excerpt: '', reason: `${words} words, over the ${basis.wordLimit} a letter reads best within.` });
    }
  }

  const seen = new Set<string>();
  const add = (flag: DraftFlag): void => {
    const key = `${flag.kind}\0${flag.excerpt.toLowerCase()}\0${flag.reason}`;
    if (!seen.has(key)) {
      seen.add(key);
      flags.push(flag);
    }
  };

  const skills = skillMatchers(basis.skills);
  const ownClaims = claimsIn(basis.ownWords, skills);
  for (const sentence of sentences(text)) {
    for (const phrase of stockPhrases) {
      for (const excerpt of sentence.match(phrase) ?? []) {
        add({ kind: 'stock_phrase', excerpt, reason: 'A stock phrase: any applicant could have written it.' });
      }
    }
    for (const claim of sentenceClaims(sentence, skills)) {
      const reason = unsupported(claim, basis, ownClaims);
      if (reason !== null) add({ kind: 'unsupported_claim', excerpt: claim.excerpt, reason });
    }
  }
  return flags;
}
