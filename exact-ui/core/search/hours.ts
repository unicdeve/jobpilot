// A port of internal/core/domain/hours.go (ADR-0025, ADR-0037): the working hours a posting asks for, as a window of UTC offsets.
// Pure TypeScript: no Exact, network, storage or clock dependency. Offsets are static, in hours without daylight saving.

import { evidence } from './signals.ts';

export type HoursStatus = '' | 'required' | 'preferred';

export interface WorkingHours {
  status: HoursStatus;
  from: number;
  to: number;
  evidence?: string;
}

export const noHours: WorkingHours = { status: '', from: 0, to: 0 };

export const hoursKnown = (h: WorkingHours): boolean =>
  h.status === 'required' || h.status === 'preferred';

// How many hours a candidate whose zone is `offset` would have to move their working day to be inside the window: zero inside it.
// Measured around the clock, so UTC+12 is an hour from UTC-11.
export function shiftFor(h: WorkingHours, offset: number): number {
  const width = h.to - h.from;
  let into = (offset - h.from) % 24;
  if (into < 0) into += 24;
  if (into <= width) return 0;
  return Math.min(into - width, 24 - into);
}

// Go's regexp is RE2: \s is ASCII-only there, and JS's also matches the no-break space the geo reader uses on purpose.
// goRe rewrites \s (and \S outside a class) into the ASCII sets.
export function goRe(source: string, flags = ''): RegExp {
  let out = '';
  let inClass = false;
  for (let i = 0; i < source.length; i++) {
    const c = source[i];
    if (c === '\\') {
      const n = source[i + 1];
      if (n === 's') out += inClass ? '\\t\\n\\f\\r ' : '[\\t\\n\\f\\r ]';
      else if (n === 'S' && !inClass) out += '[^\\t\\n\\f\\r ]';
      else out += c + n;
      i++;
      continue;
    }
    if (c === '[') inClass = true;
    else if (c === ']') inClass = false;
    out += c;
  }
  return new RegExp(out, flags);
}

// The widest window kept. A posting that names nearly every zone asks nothing.
const maxWindow = 20;
// The hours a candidate's own day is taken to run, in their own time.
const workDayStart = 9;
const workDayEnd = 17;

// Zone abbreviations postings write, at their standard offsets. Matched in capitals only: "ET" is French for "and".
const namedZones: Record<string, number> = {
  UTC: 0,
  GMT: 0,
  WET: 0,
  WEST: 0,
  BST: 0,
  CET: 1,
  CEST: 1,
  EET: 2,
  EEST: 2,
  MSK: 3,
  IST: 5.5,
  SGT: 8,
  AWST: 8,
  JST: 9,
  KST: 9,
  AEST: 10,
  AEDT: 10,
  NZST: 12,
  ET: -5,
  EST: -5,
  EDT: -5,
  CT: -6,
  CST: -6,
  CDT: -6,
  MT: -7,
  MST: -7,
  MDT: -7,
  PT: -8,
  PST: -8,
  PDT: -8,
  AKST: -9,
  HST: -10,
  BRT: -3,
  ART: -3,
};
const namedZone = (word: string): number | undefined =>
  Object.hasOwn(namedZones, word) ? namedZones[word] : undefined;

// Zones written out, longest first so "Central European" is not read as US Central.
const zoneWords: { pattern: RegExp; offset: number }[] = (
  [
    ['central european', 1],
    ['eastern european', 2],
    ['western european', 0],
    ['greenwich mean', 0],
    ['us eastern', -5],
    ['us central', -6],
    ['us mountain', -7],
    ['us pacific', -8],
    ['eastern', -5],
    ['central', -6],
    ['mountain', -7],
    ['pacific', -8],
    ['east coast', -5],
    ['west coast', -8],
    ['east', -5],
    ['west', -8],
  ] as [string, number][]
).map(([words, offset]) => {
  const pattern = words.endsWith('coast')
    ? `\\b${words}\\b`
    : `\\b${words}\\b([\\s-]+(standard\\s+)?(time|zone|timezone|time zone)|\\s*\\(|,|\\s+or\\b)`;
  return { pattern: goRe(pattern), offset };
});

const middleEast = /\bmiddle east(ern)?\b/g;
// UTC or GMT with an offset written after it: "GMT+7".
const offsetZone = () => goRe('\\b(?:UTC|GMT)\\s*([+\\-−]\\s*\\d{1,2}(?:[:.]\\d\\d)?)', 'g');
const zoneAbbreviation = /\b[A-Z]{2,4}\b/g;

// Regions postings name as time zones, as windows of standard offsets. "us" is matched in capitals only, since it is also the pronoun.
const zoneRegions: { patterns: RegExp[]; from: number; to: number }[] = [
  {
    patterns: [
      /\b(emea|europe,? (the )?middle east,? (and|&) africa|european,? middle eastern,? (and|or) african)\b/i,
    ],
    from: -1,
    to: 4,
  },
  { patterns: [/\b(western europe(an)?)\b/i], from: 0, to: 1 },
  { patterns: [/\b(europe|european)\b/i, /\bEU\b/], from: -1, to: 3 },
  { patterns: [/\b(north america|north american|namer)\b/i], from: -10, to: -3.5 },
  { patterns: [/\b(US|U\.S\.)/, /\b(united states|american)\b/i], from: -10, to: -5 },
  { patterns: [/\b(latam|latin america|south america|south american)\b/i], from: -8, to: -3 },
  { patterns: [/\b(americas|amer|north (or|and|&) south america(n)?)\b/i], from: -10, to: -3 },
  { patterns: [/\b(apac|apj|asia[- ]pacific)\b/i], from: 5, to: 13 },
  { patterns: [/\b(anz|australia|new zealand)\b/i], from: 8, to: 12 },
  { patterns: [/\b(asia|asian)\b/i], from: 5, to: 9 },
  { patterns: [/\b(africa|african)\b/i], from: -1, to: 4 },
  { patterns: [/\b(middle east|middle eastern)\b/i], from: 2, to: 4 },
  { patterns: [/\b(india|indian)\b/i], from: 5.5, to: 5.5 },
  { patterns: [/\b(canada|canadian)\b/i], from: -8, to: -3.5 },
];

// Sentences about time zones that ask nothing: a team's spread, a benefit, how people collaborate.
const hoursNoise = goRe(
  '\\b(every|all|any|multiple|different|various|several|many|disparate|other)\\b[^.]{0,30}\\btime ?zones?\\b|\\b(collaborat\\w*|work\\w*|communicat\\w*|coordinat\\w*|operat\\w*|distributed|spread|spans?|teams?|connect\\w*|async\\w*)\\b[^.]{0,25}\\bacross\\b[^.]{0,30}\\btime ?zones?\\b|\\btime ?zones?[^.]{0,20}\\b(apart|away|gap)\\b|\\bflexib\\w*\\b[^.]{0,20}\\bhours\\b|\\boutside (of )?(standard |normal |regular )?(working|business) hours\\b',
  'i',
);
// A rota rather than a working day: shown as the posting wrote it and not read.
const shiftWork = goRe(
  '\\b(daily|night|rotating|\\d+[- ]hour) shifts?\\b|\\bshifts? (being|is|will be|pattern|covers?|covering)\\b|\\bcovering weekends\\b|\\b24 ?x ?7\\b',
  'i',
);
const requireWords = /\b(only|residents?|required|requires?|must|mandatory|need to|overlap\w*)\b/i;
const preferWords =
  /\b(prefer|preferred|preferably|preference|ideally|ideal|nice to have|bonus|plus|encourage[ds]?)\b/i;
// How a requirement places the candidate: "based in", "remote in", "located in".
const regionTrigger =
  /\b(based|located|reside|residing|live|living|home[- ]based|remote|remotely|role|position|filled|held|open|available|must be|be|within|candidates|work|working|operate)\b[^.]{0,40}\b(in|within|from|across)\b|\boverlap\w*\b[^.]{0,30}\bwith\b/i;

const offsetPart = '(?:[+\\-−–]\\s*\\d{1,2}(?:[.:]\\d{1,2})?|0)';
// A range of two offsets: "UTC−6 to UTC+3", "between UTC-4 and UTC+4", "+3 / -7 UTC", "GMT to GMT+3".
const twoOffsets = goRe(
  `(?:\\b(?:UTC|GMT)\\s*(${offsetPart})?|(${offsetPart})\\s*(?:UTC|GMT)\\b)\\s*(?:to|and|/|-|–|—|through)\\s*(?:\\b(?:UTC|GMT)\\s*(${offsetPart})?|(${offsetPart})\\s*(?:UTC|GMT)?\\b)|(${offsetPart})\\s*/\\s*(${offsetPart})\\s*(?:UTC|GMT)\\b`,
);
// A zone and a margin: "CET (+/- 3 hours)", "UTC ± 2".
const zoneSpread = goRe(
  '\\b([A-Z]{2,4})\\b(?:\\s*time ?zone)?\\s*\\(?\\s*(?:\\+/-|±|\\+-|\\+/−)\\s*(\\d{1,2})',
);
const withinOf = goRe('\\bwithin\\s+(\\w+)\\s+hours?\\s+of\\s+', 'i');
// An overlap of a number of hours: "at least 4 hours overlap", "overlap of 3-4 hours", "Overlap: 6hs".
const overlapHours = goRe(
  '\\b(\\w+)(?:\\s*-\\s*\\d+)?\\s*(?:hours?|hrs?|hs)\\b[^.]{0,40}\\boverlap|\\boverlap\\b[^.]{0,20}?\\b(\\w+)(?:\\s*-\\s*\\d+)?\\s*(?:hours?|hrs?|hs)\\b',
  'i',
);
// A span of the clock in a zone: "9:00 AM – 12:00 PM PST", "between 11:00 and 16:00 UTC".
const clockHours = goRe(
  '\\b(\\d{1,2})(?:[:.](\\d\\d))?\\s*(am|pm|a\\.m\\.|p\\.m\\.)?\\s*(?:-|–|—|to|and)\\s*(\\d{1,2})(?:[:.](\\d\\d))?\\s*(am|pm|a\\.m\\.|p\\.m\\.)?\\s+',
  'i',
);
// Working in a zone's day: "working hours in CEST", "working EST hours".
const zoneHoursRe = goRe(
  '\\b(working|business|office|core)\\s+hours\\b|\\bhours\\s+(in|according to|aligned with)\\b|\\bwork(ing)?\\s+\\S+\\s+(hours|time)\\b|\\bavailab\\w*\\s+(during|in)\\b',
  'i',
);
const timeZoneWord = /\btime ?zones?\b/i;

const numberWords: Record<string, number> = {
  one: 1,
  two: 2,
  three: 3,
  four: 4,
  five: 5,
  six: 6,
  seven: 7,
  eight: 8,
};

// strconv.ParseFloat for the decimal forms a posting can hold.
const parseFloatStrict = (s: string): number | undefined =>
  /^[+-]?(\d+\.?\d*|\.\d+)$/.test(s) ? Number(s) : undefined;

const number = (word: string): number | undefined => {
  const lower = word.toLowerCase();
  if (Object.hasOwn(numberWords, lower)) return numberWords[lower];
  return parseFloatStrict(word);
};

// Reads "+3", "−5", "-3:30" or "0" as hours.
function parseOffset(input: string): number | undefined {
  let text = input.replace(/−/g, '-').replace(/–/g, '-').replace(/ /g, '');
  if (text === '') return 0;
  let sign = 1;
  if (text[0] === '-') {
    sign = -1;
    text = text.slice(1);
  } else if (text[0] === '+') {
    text = text.slice(1);
  }
  const cut = text.replace(/\./g, ':').split(':');
  const hours = cut[0];
  const minutes = cut.length > 1 ? cut.slice(1).join(':') : '';
  let h = parseFloatStrict(hours);
  if (h === undefined || h > 14) return undefined;
  if (minutes !== '') {
    const m = parseFloatStrict(minutes);
    if (m === undefined) return undefined;
    h += m / 60;
  }
  return sign * h;
}

// The standard offsets of every zone a sentence names.
function zonesIn(sentence: string): number[] {
  const offsets: number[] = [];
  // "GMT+7" is the zone seven hours east of GMT, not GMT.
  for (const m of sentence.matchAll(offsetZone())) {
    const offset = parseOffset(m[1]);
    if (offset !== undefined) offsets.push(offset);
  }
  sentence = sentence.replace(offsetZone(), '');
  for (const word of sentence.match(zoneAbbreviation) ?? []) {
    const offset = namedZone(word);
    if (offset !== undefined) offsets.push(offset);
  }
  // "Middle Eastern" is a region, not US Eastern time.
  let lower = sentence.toLowerCase().replace(middleEast, (m) => ' '.repeat(m.length));
  for (const zone of zoneWords) {
    const m = zone.pattern.exec(lower);
    if (m) {
      offsets.push(zone.offset);
      // A longer name has been read; its words are not read again.
      lower =
        lower.slice(0, m.index) + ' '.repeat(m[0].length) + lower.slice(m.index + m[0].length);
    }
  }
  return offsets;
}

interface Window {
  from: number;
  to: number;
}

function spanOf(offsets: number[]): Window | undefined {
  if (offsets.length === 0) return undefined;
  return { from: Math.min(...offsets), to: Math.max(...offsets) };
}

// The window one sentence asks for, if it asks for one.
function windowIn(sentence: string): Window | undefined {
  if (shiftWork.test(sentence)) return undefined;
  const exact = exactWindow(sentence);
  if (exact) return exact;
  if (hoursNoise.test(sentence)) return undefined;
  return namedWindow(sentence);
}

// A window a sentence states with numbers: two offsets, a zone and a margin, an overlap of hours, or a span of the clock.
function exactWindow(sentence: string): Window | undefined {
  const two = twoOffsets.exec(sentence);
  if (two) {
    const offsets: number[] = [];
    for (const part of [
      (two[1] ?? '') + (two[2] ?? ''),
      (two[3] ?? '') + (two[4] ?? ''),
      two[5] ?? '',
      two[6] ?? '',
    ]) {
      if (part === '') continue;
      const offset = parseOffset(part);
      if (offset !== undefined) offsets.push(offset);
    }
    // "GMT to GMT+3": a bare GMT is zero.
    if (offsets.length === 1 && (two[5] ?? '') === '') offsets.push(0);
    if (offsets.length === 2) return spanOf(offsets);
  }
  const spread = zoneSpread.exec(sentence);
  if (spread) {
    const offset = namedZone(spread[1]);
    if (offset !== undefined) {
      const margin = parseFloatStrict(spread[2]) ?? 0;
      return { from: offset - margin, to: offset + margin };
    }
  }
  const zones = zonesIn(sentence);
  const within = withinOf.exec(sentence);
  if (within) {
    const margin = number(within[1]);
    const after = zonesIn(sentence.slice(within.index + within[0].length));
    if (margin !== undefined && after.length > 0)
      return { from: after[0] - margin, to: after[0] + margin };
  }
  const overlap = overlapHours.exec(sentence);
  if (overlap && zones.length > 0) {
    const n = number((overlap[1] ?? '') + (overlap[2] ?? ''));
    if (n !== undefined && n >= 1 && n <= 8) {
      const span = spanOf(zones)!;
      const slack = workDayEnd - workDayStart - n;
      return { from: span.from - slack, to: span.to + slack };
    }
  }
  const clock = clockHours.exec(sentence);
  if (clock && zones.length > 0) {
    // The zone is the clock's only when it follows it: "9 AM – 12 PM PST", not "3-4 hours of overlap with CET".
    const end = clock.index + clock[0].length;
    const next = sentence.slice(end, Math.min(sentence.length, end + 14));
    const hours = clockSpan(clock);
    if (hours) {
      const after = zonesIn(next);
      if (after.length > 0) return hoursWindow(hours, after[0]);
    }
  }
  return undefined;
}

// A window a sentence names in words: zones, "working hours in CEST", or regions, "home based in EMEA time zones".
// Named zones are the narrower reading and win over regions beside them.
function namedWindow(sentence: string): Window | undefined {
  const zones = zonesIn(sentence);
  const placed =
    timeZoneWord.test(sentence) &&
    (regionTrigger.test(sentence) || requireWords.test(sentence) || preferWords.test(sentence));
  if (zones.length > 0 && (zoneHoursRe.test(sentence) || placed || requireWords.test(sentence)))
    return spanOf(zones);
  if (!placed) return undefined;
  const offsets: number[] = [];
  for (const region of zoneRegions) {
    if (region.patterns.some((p) => p.test(sentence))) offsets.push(region.from, region.to);
  }
  return spanOf(offsets);
}

// The start and end of a span of the clock, in 24-hour time, the end after the start.
function clockSpan(m: RegExpExecArray): [number, number] | undefined {
  const part = (i: number) => m[i] ?? '';
  const read = (hour: string, minute: string, meridiem: string): number | undefined => {
    let h = parseFloatStrict(hour);
    if (h === undefined || h > 24) return undefined;
    if (minute !== '') h += (parseFloatStrict(minute) ?? 0) / 60;
    switch (meridiem.replace(/\./g, '').toLowerCase()) {
      case 'pm':
        if (h < 12) h += 12;
        break;
      case 'am':
        if (h >= 12) h -= 12;
        break;
    }
    return h;
  };
  // "9 to 5" without am or pm is a working day; "9am - 5" is the same.
  const startMeridiem = part(3);
  let endMeridiem = part(6);
  if (endMeridiem === '' && startMeridiem !== '') endMeridiem = 'pm';
  const start = read(part(1), part(2), startMeridiem);
  let end = read(part(4), part(5), endMeridiem);
  if (start === undefined || end === undefined) return undefined;
  if (startMeridiem === '' && endMeridiem === '' && end < start && end < 12) end += 12;
  if (end <= start) end += 24;
  return [start, end];
}

// Where a candidate's own day must be for it to cover a span of hours in a zone. A span longer than the day is met by the one offset whose day is centred on it.
function hoursWindow(hours: [number, number], zone: number): Window {
  const from = zone + workDayStart - hours[0];
  const to = zone + workDayEnd - hours[1];
  if (from > to) {
    const mid = (from + to) / 2;
    return { from: mid, to: mid };
  }
  return { from, to };
}

// The working hours a posting asks for, sentence by sentence (ADR-0025 §1).
// A required window wins over a preferred one; two windows of the same kind are joined, since the posting accepts either.
export function readHours(sentences: string[]): WorkingHours {
  const required: WorkingHours = { ...noHours };
  const preferred: WorkingHours = { ...noHours };
  for (const sentence of sentences) {
    const found = windowIn(sentence);
    if (!found || found.to - found.from >= maxWindow) continue;
    const isPreferred = preferWords.test(sentence);
    const target = isPreferred ? preferred : required;
    if (target.status === '') {
      target.from = found.from;
      target.to = found.to;
      target.evidence = evidence(sentence);
      target.status = isPreferred ? 'preferred' : 'required';
      continue;
    }
    target.from = Math.min(target.from, found.from);
    target.to = Math.max(target.to, found.to);
  }
  return hoursKnown(required) ? required : preferred;
}

// An offset as a person reads it: "UTC+1", "UTC−5", "UTC+5:30", "UTC". An offset past +14 is written on the other side of the date line.
export function formatOffset(offset: number): string {
  if (offset > 14) offset -= 24;
  if (offset === 0) return 'UTC';
  let sign = '+';
  if (offset < 0) {
    sign = '−';
    offset = -offset;
  }
  const hours = Math.floor(offset);
  let text = `UTC${sign}${hours}`;
  const minutes = Math.round((offset - hours) * 60);
  if (minutes > 0) text += `:${minutes}`;
  return text;
}

// The hours' window: "UTC−2 to UTC+4", or "UTC+1" for one zone.
export const windowText = (h: WorkingHours): string =>
  h.from === h.to ? formatOffset(h.from) : `${formatOffset(h.from)} to ${formatOffset(h.to)}`;
