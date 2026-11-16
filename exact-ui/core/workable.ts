// A port of internal/core/domain/workable.go (ADR-0023, ADR-0037): where a job can be done from, read from its locations,
// title and description. Pure TypeScript with no Exact, network, storage or clock dependency.
// City names are read only once the host has installed the city table (geo.ts loadCities).

import * as geo from './geo.ts';
import { goRe } from './hours.ts';
import { evidence } from './signals.ts';

export type WorkableScope = 'anywhere' | 'listed' | 'unknown';

export interface Workable {
  scope: WorkableScope;
  // What the job's locations were read as, in the product's words: "EMEA", "United States", "Berlin".
  places: string[];
  // The ISO codes `places` cover, regions expanded, for a listed job.
  countries: string[];
  // The description's sentence that made the job unknown, when one did.
  note?: string;
}

export const unknownWorkable: Workable = { scope: 'unknown', places: [], countries: [] };

// Whether a candidate in `country` can take the job without moving, as far as the job says. Unknown is not a yes.
export function openTo(w: Workable, country: string): boolean {
  if (w.scope === 'anywhere') return true;
  if (w.scope === 'listed') return w.countries.includes(country.toUpperCase());
  return false;
}

export const workableKnown = (w: Workable): boolean => w.scope === 'anywhere' || w.scope === 'listed';

// The fields of a job that reading where it can be done from needs.
export interface WorkableJob { title: string; remoteType: string; locations: string[] }

// Splits a location into the alternatives it lists: "London, UK or NYC", "Berlin | München".
const alternativeSeparators = goRe('[;|/+&·]|\\s[-–—]\\s|\\s(?:or|and|oder|ou)\\s', 'i');
// Splits one alternative into its parts: "Cambridge, MA", "Zürich (Switzerland)".
const itemSeparators = /[,()[\]]/;
const anywhereWords = /\b(worldwide|world wide|anywhere|global|globally|everywhere|international|internationally|all countries|any country|any location)\b/;
// A location that lists where the job is not is unknown: exclusions are not read.
const negationWords = /\b(not in|not from|excluding|exclude|except|outside)\b/;
// A time-zone requirement, which is not a place.
const timeZoneWords = /\b(time ?zones?|timezones?|hours|utc|gmt|cet|cest|est|edt|pst|pdt|pt|et|ct|overlap)\b/;
// How or on what terms a job is done, not where.
const noiseWords = new Set([
  'remote', 'remotely', 'fully', 'full', '100', 'only', 'hybrid', 'onsite', 'on', 'site', 'office', 'in',
  'based', 'candidates', 'applicants', 'residents', 'citizens', 'persons', 'preferred', 'required', 'work', 'from',
  'home', 'wfh', 'flexible', 'location', 'locations', 'the', 'within', 'remoto', 'distance', 'a', 'poste',
]);

interface LocationReading { anywhere: boolean; places: geo.Place[] }

// Reads one location as a source wrote it. A location that lists where a job is not is unread as a whole.
function readLocation(location: string): LocationReading {
  const folded = geo.fold(location);
  if (negationWords.test(folded)) return { anywhere: false, places: [] };
  const reading: LocationReading = { anywhere: anywhereWords.test(folded), places: [] };
  for (const alternative of keepCompoundsWhole(location).split(alternativeSeparators)) {
    reading.places.push(...readAlternative(alternative));
  }
  return reading;
}

const escapeRe = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// Matches any country or region name a location would be split inside, in any case and spacing.
let compoundCache: RegExp | undefined;
const compoundName = (): RegExp => {
  if (compoundCache) return compoundCache;
  const patterns = geo.compounds().map((name) =>
    name.split(/[ ,()&/']+/).filter((w) => w !== '').map(escapeRe).join("[\\t\\n\\f\\r ,()&/']+"));
  compoundCache = new RegExp(`\\b(${patterns.join('|')})\\b\\)?`, 'gi');
  return compoundCache;
};

// Joins the words of a compound name with no-break spaces, which no separator splits at and which folding reads as spaces.
function keepCompoundsWhole(location: string): string {
  return location.replace(compoundName(), (name) => name.replace(/[ ,()&/]/g, '\u00a0'));
}

// Reads one alternative a location lists. What else the alternative names says which city a name means.
function readAlternative(alternative: string): geo.Place[] {
  let read: geo.Place[] = [];
  const states = new Set<string>();
  for (const item of alternative.split(itemSeparators)) {
    const country = Object.hasOwn(subdivisionCodes, item.trim()) ? subdivisionCodes[item.trim()] : undefined;
    if (country) {
      states.add(country);
      continue;
    }
    read.push(...readSegment(item));
  }
  read = placeStates(read, states);

  const named = new Set<string>();
  for (const place of read) {
    if (place.kind !== 'city') for (const c of place.countries) named.add(c);
  }
  // Cities are placed once the whole alternative is read, and kept in the order it names them.
  const places: geo.Place[] = [];
  for (let place of read) {
    if (place.kind === 'city') {
      const placed = placeCity(place, named, states);
      if (!placed) continue;
      place = placed;
    }
    places.push(place);
  }
  return places;
}

// Reads a country's name that is also a state's as the state when the rest of the alternative is in the state's country.
function placeStates(read: geo.Place[], states: Set<string>): geo.Place[] {
  read.forEach((place, i) => {
    if (place.kind !== 'country') return;
    const country = geo.subdivisionOf(place.name);
    if (!country) return;
    if (states.has(country) || elsewhereIn(read, i, country, place.countries[0])) {
      const state = geo.countryPlace(country);
      if (state) read[i] = state;
    }
  });
  return read;
}

// Whether a place other than read[skip] is in `country`: a country or region that holds it, or a city that is there and not in `other`.
function elsewhereIn(read: geo.Place[], skip: number, country: string, other: string): boolean {
  for (let i = 0; i < read.length; i++) {
    if (i === skip) continue;
    const place = read[i];
    if (place.kind !== 'city') {
      if (place.countries.includes(country)) return true;
      continue;
    }
    const all = [...place.countries, ...place.elsewhere];
    if (all.includes(country) && !all.includes(other)) return true;
  }
  return false;
}

// Picks the city a name means from what the rest of its alternative names.
function placeCity(city: geo.Place, named: Set<string>, states: Set<string>): geo.Place | undefined {
  if (named.size > 0) return cityAmong(city, named);
  return cityAmong(city, states) ?? city;
}

function cityAmong(city: geo.Place, countries: Set<string>): geo.Place | undefined {
  if (countries.has(city.countries[0])) return city;
  for (const country of city.elsewhere) {
    if (countries.has(country)) return geo.placeIn(city, country);
  }
  return undefined;
}

// The state and province codes postings write after a city, in capitals, as in "Seattle, WA".
const subdivisionCodes: Record<string, string> = (() => {
  const codes: Record<string, string> = {};
  for (const code of `AL AK AZ AR CA CO CT DE DC FL GA HI ID IL IN IA KS KY LA ME MD
    MA MI MN MS MO MT NE NV NH NJ NM NY NC ND OH OK OR PA RI SC SD TN TX UT VT VA WA WV WI WY`.split(/\s+/)) codes[code] = 'US';
  for (const code of 'AB BC MB NB NL NS ON PE QC SK'.split(' ')) codes[code] = 'CA';
  return codes;
})();

// Reads one listed place. The whole segment, less the words that are not places, is tried first; then its runs of words.
function readSegment(segment: string): geo.Place[] {
  const folded = geo.fold(segment);
  if (folded === '' || timeZoneWords.test(folded)) return [];
  const words = folded.split(' ').filter((w) => w !== '' && !noiseWords.has(w));
  if (words.length === 0) return [];
  const place = geo.lookup(words.join(' '), true);
  if (place) return [place];
  return placesInWords(words, undefined, true);
}

// Finds places in a run of words, longest first, each word used once; cities only when `cities` is set.
// `original` is the words as they were written, so that "us" the pronoun is not read as "US" the country.
function placesInWords(words: string[], original: string[] | undefined, cities: boolean): geo.Place[] {
  const places: geo.Place[] = [];
  for (let start = 0; start < words.length;) {
    let matched = 0;
    for (let n = Math.min(4, words.length - start); n > 0; n--) {
      const candidate = words.slice(start, start + n).join(' ');
      if (n === 1 && candidate === 'us' && original && original[start] === 'us') continue;
      const place = geo.lookup(candidate, cities);
      if (place) {
        places.push(place);
        matched = n;
        break;
      }
    }
    if (matched === 0) matched = 1;
    start += matched;
  }
  return places;
}

// Derives where a job can be done from, from its locations and remote type, and checks the result against its description.
export function readWorkable(job: WorkableJob, sentences: string[]): Workable {
  const inPlace = job.remoteType === 'onsite' || job.remoteType === 'hybrid';
  let anywhere = false;
  const places: geo.Place[] = [];
  for (const location of job.locations) {
    const reading = readLocation(location);
    // An onsite job is done where its office is, whatever else its location says.
    if (reading.anywhere && !inPlace) anywhere = true;
    places.push(...reading.places);
  }

  let workable: Workable = { scope: 'unknown', places: [], countries: [] };
  if (anywhere) {
    workable.scope = 'anywhere';
  } else if (places.length > 0) {
    const [names, countries] = placeNames(places);
    workable = { scope: 'listed', places: names, countries };
  }
  if (inPlace || !workableKnown(workable)) return workable;

  // The title and the description can make a job less certain, never more (ADR-0023 §1).
  const named = countriesIn(job.title);
  if (named.length > 0 && (workable.scope === 'anywhere' || !overlaps(named, workable.countries))) {
    return { scope: 'unknown', places: [], countries: [], note: evidence(job.title) };
  }
  const note = contradiction(workable, sentences);
  if (note !== undefined) return { scope: 'unknown', places: [], countries: [], note };
  return workable;
}

// The places' names and their countries, each once and in order; the countries sorted.
function placeNames(places: geo.Place[]): [string[], string[]] {
  const names: string[] = [];
  const countries: string[] = [];
  for (const place of places) {
    if (!names.includes(place.name)) names.push(place.name);
    for (const c of place.countries) if (!countries.includes(c)) countries.push(c);
  }
  countries.sort();
  return [names, countries];
}

// Phrases that require the candidate to be somewhere. They contradict a job labelled anywhere whether or not a place is named.
const residencePhrases = goRe([
  '\\b(must|need to|needs to|required to|have to|has to|should)( currently)?( be)? (located|based|residing|reside|living|live|resident)( in| within| out of)\\b',
  '\\b(based|located|residing) in (the )?[a-z ]{2,30} only\\b',
  '\\bbased (candidates |applicants |residents |talent )?only\\b',
  '\\bonly (open to|accepting|considering|hiring|available to)\\b.{0,40}\\b(in|from|based|located|residing|living)\\b',
  '\\bopen to (candidates|applicants|residents) (residing|located|based|living) in\\b',
  '\\b(citizens?|persons|nationals) only\\b',
  '\\btax residen(cy|t)\\b',
].join('|'));
// Phrases that restrict where a candidate may work only when they name a place.
const authorizationPhrases = goRe([
  '\\b(authori[sz]ed|eligible|legally (able|allowed|permitted)|permitted|entitled|able) to work (in|within|for)\\b',
  '\\bright to work in\\b',
  '\\bremote(ly)? (within|in|from) (the )?[a-z]',
].join('|'));

// The first description sentence that restricts where a job labelled anywhere or listed can be done from, beyond what its label says.
// Returns its evidence, or undefined when none does.
function contradiction(workable: Workable, sentences: string[]): string | undefined {
  for (const sentence of sentences) {
    const folded = geo.fold(sentence);
    const residence = residencePhrases.test(folded);
    const authorization = authorizationPhrases.test(folded);
    if (!residence && !authorization) continue;
    // "must be located within three hours of CET" is a time zone.
    if (timeZoneWords.test(folded)) continue;
    const named = countriesIn(sentence);
    if (named.length === 0 && !residence) continue;
    if (workable.scope === 'listed' && named.length > 0 && overlaps(named, workable.countries)) continue;
    return evidence(sentence);
  }
  return undefined;
}

// Every country a sentence names, as a country or a region. Cities are not read in prose: "Nice to have" is not in France.
function countriesIn(sentence: string): string[] {
  const folded: string[] = [];
  const original: string[] = [];
  for (const word of sentence.split(/[^.'\p{L}\p{Nd}]+/u)) {
    const f = geo.fold(word);
    if (f === '') continue;
    folded.push(f);
    original.push(word.replace(/\./g, ''));
  }
  return placeNames(placesInWords(folded, original, false))[1];
}

const overlaps = (countries: string[], of: string[]): boolean => countries.some((c) => of.includes(c));
