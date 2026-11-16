// A port of the Go core's relocation and visa-sponsorship reading (internal/core/domain/offers.go, plus the
// Sentences and evidence helpers it uses from signals.go), ADR-0037, ADR-0023 section 5.
// An offer is a label for the candidate, always shown with the sentence it came from, never a claim that a package exists.
// Pure: no Exact, network or storage dependency. The regexps are RE2 in Go and ECMAScript here;
// every one is written without lookaround or backreferences, so both engines read them the same (\b, \w and \s are ASCII-only in Go,
// so \s is spelled out below).

export type OfferStatus = 'offered' | 'mentioned' | 'declined';

// status is '' for a posting that does not mention the help.
export interface Offer {
  status: OfferStatus | '';
  evidence: string;
}

// What a search that includes relocation keeps: offered, or mentioned without refusing.
export const offerAvailable = (o: Offer): boolean =>
  o.status === 'offered' || o.status === 'mentioned';

// ---- text helpers (geo.Fold, Sentences, evidence) ----

// geo.Fold: lower case, accents removed, dots dropped, anything else that is not a letter or digit read as a space.
export function fold(s: string): string {
  const stripped = s
    .normalize('NFKD')
    .replace(/\p{Mn}/gu, '')
    .normalize('NFC');
  let out = '';
  let space = false;
  for (const ch of stripped.toLowerCase()) {
    if (ch === '.') continue;
    if (/[\p{L}\p{Nd}]/u.test(ch)) {
      if (space && out.length > 0) out += ' ';
      space = false;
      out += ch;
    } else {
      space = true;
    }
  }
  return out;
}

const namedEntities: Record<string, string> = {
  amp: '&',
  lt: '<',
  gt: '>',
  quot: '"',
  apos: "'",
  nbsp: ' ',
  euro: '€',
  pound: '£',
  yen: '¥',
  copy: '©',
  reg: '®',
  ndash: '–',
  mdash: '—',
  hellip: '…',
  lsquo: '‘',
  rsquo: '’',
  ldquo: '“',
  rdquo: '”',
  bull: '•',
  middot: '·',
  auml: 'ä',
  ouml: 'ö',
  uuml: 'ü',
  Auml: 'Ä',
  Ouml: 'Ö',
  Uuml: 'Ü',
  szlig: 'ß',
  eacute: 'é',
  egrave: 'è',
  agrave: 'à',
  aacute: 'á',
  ccedil: 'ç',
  ntilde: 'ñ',
};

// html.UnescapeString, for the entities a job posting uses (not the whole HTML5 table).
const unescapeHtml = (s: string): string =>
  s.replace(
    /&(?:#[xX]([0-9a-fA-F]+)|#([0-9]+)|([A-Za-z][A-Za-z0-9]*));/g,
    (whole, hex, dec, name) => {
      if (hex !== undefined || dec !== undefined) {
        const code = hex !== undefined ? parseInt(hex, 16) : parseInt(dec, 10);
        return code > 0 && code <= 0x10ffff && !(code >= 0xd800 && code <= 0xdfff)
          ? String.fromCodePoint(code)
          : '�';
      }
      return namedEntities[name] ?? whole;
    },
  );

const htmlTag = /<[^>]*>/g;
const markdownLink = /\[([^\]]*)\]\([^)]*\)/g;
const markdownMarks = /\*\*|__|\\|#|`/g;
const abbreviations: Record<string, string> = {
  'U.S.A.': 'USA',
  'U.S.': 'US',
  'U.K.': 'UK',
  'U.A.E.': 'UAE',
  'e.g.': 'eg',
  'i.e.': 'ie',
  'etc.': 'etc',
  'approx.': 'approx',
  'incl.': 'incl',
};
const abbreviationPattern =
  /U\.S\.A\.|U\.S\.|U\.K\.|U\.A\.E\.|e\.g\.|i\.e\.|etc\.|approx\.|incl\./g;
// Also splits at " | ", which is how a Hacker News posting separates its fields.
const sentenceEnd =
  /[.!?]+(?:[\t\n\f\r ]+|$)|\n+|[\t\n\f\r ][-*•][\t\n\f\r ]|[\t\n\f\r ]\|[\t\n\f\r ]/;
const whitespace = /[\t\n\f\r ]+/g;

const maxEvidence = 280;

// A posting as plain-text sentences: HTML and markdown removed, a list item or a line its own sentence.
export function sentences(description: string): string[] {
  // Descriptions from some sources arrive escaped twice.
  let text = unescapeHtml(unescapeHtml(description));
  text = text.replace(htmlTag, '\n').replace(markdownLink, '$1').replace(markdownMarks, '');
  text = text.replace(abbreviationPattern, (m) => abbreviations[m]!);
  const out: string[] = [];
  for (const raw of text.split(sentenceEnd)) {
    let part = raw.replace(whitespace, ' ').trim();
    part = part.replace(/^[-*•> ]+/, '');
    if (part !== '') out.push(part);
  }
  return out;
}

// A sentence as kept and shown: whole, unless too long to read at a glance.
// Go counts bytes; this counts UTF-16 units, so a non-ASCII sentence keeps slightly more.
export function evidence(sentence: string): string {
  if (sentence.length <= maxEvidence) return sentence;
  let cut = sentence.slice(0, maxEvidence).lastIndexOf(' ');
  if (cut <= 0) cut = maxEvidence;
  return `${sentence.slice(0, cut)}…`;
}

// ---- the rules ----

const helpWords = String.raw`(offer|offering|provide|providing|support|supporting|sponsor|sponsoring|cover|covering|assist|consider|accommodate|help|include|relocat\w*|visas?|sponsorship)\b`;

// Splits a sentence where it can turn. Non-capturing, so split() keeps no separators.
const clauseBreaks = /\b(?:but|however|although|though|whereas)\b|;/i;

// Refusals, read on folded text, so "don't" is "don t". A negation counts only where it governs the help itself.
const declinedCues = new RegExp(
  [
    String.raw`\b(unable to|cannot|can not|can t|cant|not able to|will not|won t|would not|not currently|no longer)( \w+){0,5} ` +
      helpWords,
    String.raw`\b(are not|is not|do not|does not|don t|doesn t)( \w+){0,2} ` + helpWords,
    String.raw`\b(relocation|sponsorship|visas?)( \w+){0,6} (is|are|will|can|would|be)( \w+)? not\b`,
    String.raw`\bnot (be )?(provided|offered|available|possible|eligible|included|covered|supported)\b`,
    String.raw`\bno (visa|visas|relocation|sponsorship|work permit)\b`,
    String.raw`\bwithout (\w+ ){0,4}(sponsorship|sponsor|relocat\w*|visas?)\b`,
  ].join('|'),
);

const conditionalCues =
  /\b(may|might|could|possibly|potentially|case by case|position dependent|depending|dependent|where (possible|applicable)|if (needed|required|applicable|eligible)|for eligible|eligible candidates|subject to|transfer only|can be discussed|negotiable|in some cases)\b/;

const relocationTerm = /\brelocat(e|ion|ing)\b/;
const visaTerm = /\bvisas?\b/;
const sponsorTerm = /\bsponsor(s|ed|ing|ship)?\b/;
// Sponsorship that is not a visa's: "mentor, sponsor and develop senior engineers", "company-sponsored dinners".
const sponsorOtherwise =
  /\b(company|employer|team|firm) sponsored\b|\bsponsored (dinners?|events?|memberships?|lunch(es)?|activities|retirement)\b|\bmentor\w* sponsor\b|\bsponsor and develop\b/;
// Sponsorship in the words of work permission.
const sponsorWork = new RegExp(
  [
    String.raw`\b(work permit|right to work|work authori[sz]ation|authori[sz]ed to work|immigration|h 1 ?b|blue card|green card|relocat\w*|employment)\b`,
    String.raw`\b(offer|offers|offering|provide|provides|providing|support|supports|require|requires|requiring|need|needs|able to) (\w+ ){0,2}sponsor`,
    String.raw`\bsponsorship (is |are )?(available|offered|provided|possible|yes)\b`,
    String.raw`\b(no|without) (\w+ ){0,2}sponsorship\b`,
    String.raw`\bsponsor (visas|you|your)\b`,
  ].join('|'),
);

const relocationOffered = new RegExp(
  [
    String.raw`\brelocation (package|support|assistance|allowance|bonus|stipend|budget|help|benefits?|costs?|expenses|reimbursement)\b`,
    String.raw`\brelocation (\w+ ){0,3}(covered|provided|offered|paid|available|included)\b`,
    String.raw`\b(offer|offers|offering|provide|provides|providing|cover|covers|support|supports|help|helps|assist|assists|pay for|pays for) (\w+ ){0,4}relocat`,
    String.raw`\bfull relocation\b`,
  ].join('|'),
);

const visaOffered = new RegExp(
  [
    String.raw`\b(visa )?sponsorship (is )?(available|provided|offered|yes)\b`,
    String.raw`\bvisa (sponsorship|support|assistance|help)\b`,
    String.raw`\b(offer|offers|offering|provide|provides|providing|support|supports) (\w+ ){0,3}(visa|sponsorship)\b`,
    String.raw`\bwe (can|will|do) sponsor\b`,
    String.raw`\bsponsor (visas|your visa)\b`,
  ].join('|'),
);

export interface OfferTopic {
  // Whether a clause is about this kind of help at all.
  about: (clause: string) => boolean;
  // Phrasing that states an offer.
  offered: RegExp;
}

export const relocationTopic: OfferTopic = {
  about: (c) => relocationTerm.test(c),
  offered: relocationOffered,
};

export const visaTopic: OfferTopic = {
  about: (c) =>
    visaTerm.test(c) || (sponsorTerm.test(c) && !sponsorOtherwise.test(c) && sponsorWork.test(c)),
  offered: visaOffered,
};

// A refusal wins: a posting that refuses in one sentence and lists benefits "that may include relocation" in another has refused.
const offerRank: Record<string, number> = { declined: 3, offered: 2, mentioned: 1, '': 0 };

// A clause's status: a refusal anywhere declines; a conditional or a question only mentions; an offer stated plainly offers.
function classifyClause(clause: string, topic: OfferTopic, question: boolean): OfferStatus {
  if (declinedCues.test(clause)) return 'declined';
  if (question || conditionalCues.test(clause)) return 'mentioned';
  if (topic.offered.test(clause)) return 'offered';
  return 'mentioned';
}

// Reads a posting's sentences for one kind of help, clause by clause, keeping the strongest status with its sentence.
export function readOffer(sentenceList: string[], topic: OfferTopic): Offer {
  let best: Offer = { status: '', evidence: '' };
  for (const sentence of sentenceList) {
    const question = sentence.trim().endsWith('?');
    // Split before folding: folding turns the semicolon into a space.
    for (const raw of sentence.split(clauseBreaks)) {
      const clause = fold(raw);
      if (!topic.about(clause)) continue;
      const status = classifyClause(clause, topic, question);
      if (offerRank[status]! > offerRank[best.status]!)
        best = { status, evidence: evidence(sentence) };
    }
  }
  return best;
}

// What a posting says about relocation and visa sponsorship.
export function readOffers(description: string): { relocation: Offer; visaSponsorship: Offer } {
  const s = sentences(description);
  return { relocation: readOffer(s, relocationTopic), visaSponsorship: readOffer(s, visaTopic) };
}
