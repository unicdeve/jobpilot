// A port of the Go core's posting reader (internal/core/domain/signals.go), ADR-0037: Sentences, evidence and SignalsVersion.
// It has no Exact, network or storage dependency, so it runs the same in the app's JS engine and under `bun test`.
// readSignals assembles readWorkable (workable.ts), readOffer (offers.ts) and readHours (hours.ts).
import { readHours, type WorkingHours } from './hours.ts';
import { readOffer, relocationTopic, visaTopic, type Offer } from './offers.ts';
import { readWorkable, type Workable, type WorkableJob } from './workable.ts';
// Go's \s, strings.Fields and strings.TrimSpace differ from JS's, so the classes below are spelled out.

// The version of the reading ReadSignals does; raise it with the Go constant.
export const signalsVersion = 3;

// RE2's \s: ASCII whitespace only.
const WS = '[\\t\\n\\f\\r ]';
// unicode.IsSpace, which is what strings.Fields and strings.TrimSpace split and trim on (JS's \s also has U+FEFF and lacks U+0085).
export const GO_SPACE = '\\t\\n\\v\\f\\r \\u0085\\u00A0\\u1680\\u2000-\\u200A\\u2028\\u2029\\u202F\\u205F\\u3000';

// strings.Fields
export const fields = (s: string): string[] => s.split(new RegExp(`[${GO_SPACE}]+`)).filter((w) => w !== '');
// strings.TrimSpace
export const trimSpace = (s: string): string => s.replace(new RegExp(`^[${GO_SPACE}]+|[${GO_SPACE}]+$`, 'g'), '');

const htmlTag = /<[^>]*>/g;
const markdownLink = /\[([^\]]*)\]\([^)]*\)/g;
const markdownMarks = /\*\*|__|\\|#|`/g;
const abbreviationMap: Record<string, string> = {
  'U.S.A.': 'USA', 'U.S.': 'US', 'U.K.': 'UK', 'U.A.E.': 'UAE',
  'e.g.': 'eg', 'i.e.': 'ie', 'etc.': 'etc', 'approx.': 'approx', 'incl.': 'incl',
};
const abbreviations = /U\.S\.A\.|U\.S\.|U\.K\.|U\.A\.E\.|e\.g\.|i\.e\.|etc\.|approx\.|incl\./g;
// Also splits at " | ", which is how a Hacker News posting separates its fields.
const sentenceEnd = new RegExp(`[.!?]+(?:${WS}+|$)|\\n+|${WS}[-*•]${WS}|${WS}\\|${WS}`);
const whitespace = new RegExp(`${WS}+`, 'g');
const leadingMarks = /^[-*•> ]+/;

export const maxEvidence = 280;

// html.UnescapeString for what postings contain: numeric references and the common named ones (Go knows all ~2200 HTML5 names).
const named: Record<string, string> = {
  amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ', copy: '©', reg: '®', trade: '™', euro: '€', pound: '£', yen: '¥', cent: '¢',
  ndash: '–', mdash: '—', hellip: '…', bull: '•', middot: '·', lsquo: '‘', rsquo: '’', ldquo: '“', rdquo: '”', laquo: '«', raquo: '»',
  sect: '§', deg: '°', times: '×', eacute: 'é', egrave: 'è', agrave: 'à', aacute: 'á', uuml: 'ü', ouml: 'ö', auml: 'ä', szlig: 'ß', ccedil: 'ç', ntilde: 'ñ',
};
// Names Go also reads without their semicolon.
const legacy = new Set(['amp', 'lt', 'gt', 'quot', 'nbsp', 'copy', 'reg']);
// Windows-1252 code points that HTML reads in place of C1 controls.
const cp1252: Record<number, string> = {
  0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡', 0x88: 'ˆ', 0x89: '‰', 0x8a: 'Š', 0x8b: '‹', 0x8c: 'Œ', 0x8e: 'Ž',
  0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™', 0x9a: 'š', 0x9b: '›', 0x9c: 'œ', 0x9e: 'ž', 0x9f: 'Ÿ',
};

const fromCodePoint = (n: number): string => {
  if (cp1252[n] !== undefined) return cp1252[n]!;
  if (n === 0 || (n >= 0xd800 && n <= 0xdfff) || n > 0x10ffff) return '�';
  return String.fromCodePoint(n);
};

export const unescapeHtml = (s: string): string =>
  s.replace(/&(?:#([xX][0-9a-fA-F]+|[0-9]+);?|([A-Za-z][A-Za-z0-9]*)(;?))/g, (all, num: string | undefined, name: string | undefined, semi: string | undefined) => {
    if (num !== undefined) {
      const n = num[0] === 'x' || num[0] === 'X' ? parseInt(num.slice(1), 16) : parseInt(num, 10);
      return Number.isFinite(n) ? fromCodePoint(n) : '�';
    }
    if (name !== undefined && semi === ';' && named[name] !== undefined) return named[name]!;
    if (name !== undefined && semi === '') {
      const prefix = [...legacy].find((l) => name.startsWith(l));
      if (prefix !== undefined) return named[prefix]! + name.slice(prefix.length);
    }
    return all;
  });

// A posting as its sentences, in plain text: HTML and markdown removed, a list item or a line its own sentence.
export function sentences(description: string): string[] {
  let text = unescapeHtml(description);
  // Descriptions from some sources arrive escaped twice.
  text = unescapeHtml(text);
  text = text.replace(htmlTag, '\n');
  text = text.replace(markdownLink, '$1');
  text = text.replace(markdownMarks, '');
  text = text.replace(abbreviations, (m) => abbreviationMap[m]!);

  const out: string[] = [];
  for (const raw of text.split(sentenceEnd)) {
    const part = trimSpace(raw.replace(whitespace, ' ')).replace(leadingMarks, '');
    if (part !== '') out.push(part);
  }
  return out;
}

// A sentence as it is kept and shown: whole, unless it is too long to read at a glance.
// Go measures bytes; this measures UTF-16 units, so a cut never splits a character.
export function evidence(sentence: string): string {
  if (sentence.length <= maxEvidence) return sentence;
  let cut = sentence.slice(0, maxEvidence).lastIndexOf(' ');
  if (cut <= 0) cut = maxEvidence;
  return `${sentence.slice(0, cut)}…`;
}

// The fields of a job that reading its signals needs.
export interface SignalJob extends WorkableJob { descriptionMarkdown: string }

export interface Signals {
  workable: Workable;
  relocation: Offer;
  visaSponsorship: Offer;
  hours: WorkingHours;
  signalsVersion: number;
}

// What a job says about itself beyond its fields: where it can be done from, relocation, visa sponsorship and working hours (ADR-0023, ADR-0025).
// It reads only what the job already holds, and no model. Go sets these on the job; here the caller merges the result.
export function readSignals(job: SignalJob): Signals {
  const s = sentences(job.descriptionMarkdown);
  return {
    workable: readWorkable(job, s),
    relocation: readOffer(s, relocationTopic),
    visaSponsorship: readOffer(s, visaTopic),
    hours: readHours(s),
    signalsVersion,
  };
}
