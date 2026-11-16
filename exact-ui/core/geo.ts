// A port of internal/core/domain/geo (ADR-0023 §2, ADR-0037): the countries, regions and cities a job's location is read against.
// Pure TypeScript with no Exact, network or storage dependency.
// Countries and time zones are generated into geo-data.ts. The city table (cities.tsv.gz in the Go package, about 62,000 lines)
// is not bundled: the host loads it once with loadCities(tsv), and until it does no city is a place, which only leaves jobs "unknown".

import { countries as countryList, timeZones, type Country } from './geo-data.ts';

export type { Country };
export type Kind = 'country' | 'region' | 'city';

// What a place name resolved to. `countries` is every country the place covers: one for a country or a city, many for a region.
// For a city, `elsewhere` is the other countries with a city of that name, most populous first.
export interface Place {
  kind: Kind;
  name: string;
  countries: string[];
  elsewhere: string[];
}

// The one way a place name is reduced to a lookup key: lower case, accents removed, dots dropped,
// and anything else that is not a letter or a digit read as a space.
export function fold(s: string): string {
  const stripped = s.normalize('NFKD').replace(/\p{Mn}/gu, '').normalize('NFC');
  let out = '';
  let space = false;
  for (const r of stripped.toLowerCase()) {
    if (r === '.') continue;
    if (/[\p{L}\p{Nd}]/u.test(r)) {
      if (space && out.length > 0) out += ' ';
      space = false;
      out += r;
    } else {
      space = true;
    }
  }
  return out;
}

// The city of this name in `country`, if the place is a city that has one.
export function placeIn(p: Place, country: string): Place | undefined {
  if (p.kind !== 'city') return undefined;
  if (p.countries.length === 1 && p.countries[0] === country) return p;
  if (p.elsewhere.includes(country)) return { kind: 'city', name: p.name, countries: [country], elsewhere: [] };
  return undefined;
}

// The other ways postings and people write a country. Two-letter codes are listed only where postings use them as words.
const countryAliases: Record<string, string[]> = {
  AE: ['UAE', 'U.A.E.', 'Emirates'],
  AT: ['Österreich'],
  BE: ['Belgique', 'België'],
  BN: ['Brunei Darussalam'],
  BO: ['Bolivia (Plurinational State of)', 'Bolivia, Plurinational State of'],
  BR: ['Brasil'],
  CD: ['DRC', 'DR Congo', 'Democratic Republic of the Congo', 'Congo (Democratic Republic of the)', 'Congo, The Democratic Republic of the', 'Congo, Democratic Republic of the', 'Congo-Kinshasa'],
  CG: ['Congo', 'Republic of Congo', 'Congo-Brazzaville'],
  CH: ['Schweiz', 'Suisse', 'Svizzera'],
  CI: ["Côte d'Ivoire"],
  CV: ['Cabo Verde'],
  CZ: ['Czech Republic'],
  DE: ['Deutschland'],
  DK: ['Danmark'],
  ES: ['España'],
  FI: ['Suomi'],
  FM: ['Micronesia (Federated States of)', 'Micronesia, Federated States of'],
  GB: ['UK', 'U.K.', 'Great Britain', 'Britain', 'England', 'Scotland', 'Wales', 'Northern Ireland', 'United Kingdom of Great Britain and Northern Ireland'],
  IE: ['Republic of Ireland'],
  IR: ['Iran (Islamic Republic of)', 'Iran, Islamic Republic of'],
  IT: ['Italia'],
  KP: ["Korea (Democratic People's Republic of)", "Korea, Democratic People's Republic of"],
  KR: ['Korea', 'Republic of Korea', 'Korea (Republic of)', 'Korea, Republic of'],
  LA: ["Lao People's Democratic Republic"],
  MD: ['Moldova (Republic of)', 'Moldova, Republic of'],
  MK: ['Macedonia (the former Yugoslav Republic of)'],
  MX: ['México'],
  NL: ['Netherlands', 'Holland'],
  NO: ['Norge'],
  PL: ['Polska'],
  PS: ['Palestine', 'Palestine, State of'],
  RU: ['Russian Federation'],
  SE: ['Sverige'],
  SY: ['Syrian Arab Republic'],
  SZ: ['Eswatini'],
  TR: ['Türkiye'],
  TW: ['Taiwan, Province of China'],
  TZ: ['Tanzania, United Republic of'],
  US: ['USA', 'US', 'U.S.', 'U.S.A.', 'United States of America', 'the US', 'the United States'],
  VA: ['Holy See'],
  VE: ['Venezuela (Bolivarian Republic of)', 'Venezuela, Bolivarian Republic of'],
  VG: ['Virgin Islands (British)', 'Virgin Islands, British'],
  VI: ['Virgin Islands (U.S.)', 'Virgin Islands, U.S.'],
  VN: ['Viet Nam'],
};

// States and provinces postings name in place of their country. One that has a country's name is the country when looked up alone.
const subdivisions: Record<string, string[]> = {
  US: [
    'Alabama', 'Alaska', 'Arizona', 'Arkansas', 'California', 'Colorado', 'Connecticut',
    'Delaware', 'District of Columbia', 'Washington DC', 'Florida', 'Georgia', 'Hawaii', 'Idaho',
    'Illinois', 'Indiana', 'Iowa', 'Kansas', 'Kentucky', 'Louisiana', 'Maine', 'Maryland',
    'Massachusetts', 'Michigan', 'Minnesota', 'Mississippi', 'Missouri', 'Montana', 'Nebraska',
    'Nevada', 'New Hampshire', 'New Jersey', 'New Mexico', 'New York State', 'North Carolina',
    'North Dakota', 'Ohio', 'Oklahoma', 'Oregon', 'Pennsylvania', 'Rhode Island',
    'South Carolina', 'South Dakota', 'Tennessee', 'Texas', 'Utah', 'Vermont', 'Virginia',
    'Washington State', 'West Virginia', 'Wisconsin', 'Wyoming',
    'Bay Area', 'San Francisco Bay Area', 'Silicon Valley',
  ],
  CA: [
    'Ontario', 'Quebec', 'Québec', 'British Columbia', 'Alberta', 'Manitoba', 'Saskatchewan',
    'Nova Scotia', 'New Brunswick', 'Newfoundland and Labrador', 'Prince Edward Island',
  ],
  DE: [
    'Bayern', 'Bavaria', 'Baden-Württemberg', 'Nordrhein-Westfalen', 'North Rhine-Westphalia',
    'Hessen', 'Hesse', 'Niedersachsen', 'Lower Saxony', 'Sachsen', 'Saxony',
  ],
  AU: ['New South Wales', 'Queensland', 'Western Australia', 'South Australia', 'Tasmania'],
};

// The short names postings write for a city.
const cityAbbreviations: Record<string, string> = { NYC: 'New York City', SF: 'San Francisco' };

interface RegionDefinition {
  name: string;
  aliases?: string[];
  continents?: string[];
  include?: string[];
  exclude?: string[];
}

const middleEastCodes = ['AE', 'BH', 'EG', 'IL', 'IQ', 'IR', 'JO', 'KW', 'LB', 'OM', 'PS', 'QA', 'SA', 'SY', 'TR', 'YE'];
const northAfrica = ['DZ', 'EG', 'EH', 'LY', 'MA', 'SD', 'TN'];
const centralAmerica = ['BZ', 'CR', 'GT', 'HN', 'NI', 'PA', 'SV'];
const euEEA = [
  'AT', 'BE', 'BG', 'CY', 'CZ', 'DE', 'DK', 'EE', 'ES', 'FI', 'FR', 'GR', 'HR', 'HU', 'IE',
  'IT', 'LT', 'LU', 'LV', 'MT', 'NL', 'PL', 'PT', 'RO', 'SE', 'SI', 'SK',
  // The EEA and Switzerland, whom "EU work authorisation" also admits.
  'IS', 'LI', 'NO', 'CH',
];

// The regions postings were measured to name. North America includes Mexico (ADR-0023 §1).
export const regionDefinitions: RegionDefinition[] = [
  { name: 'Europe', aliases: ['European'], continents: ['EU'] },
  { name: 'EU', aliases: ['European Union', 'EEA', 'EU/EEA'], include: euEEA },
  { name: 'EMEA', continents: ['EU', 'AF'], include: middleEastCodes },
  { name: 'Middle East', include: middleEastCodes },
  { name: 'MENA', include: [...middleEastCodes, ...northAfrica] },
  { name: 'GCC', include: ['AE', 'BH', 'KW', 'OM', 'QA', 'SA'] },
  { name: 'Africa', continents: ['AF'] },
  { name: 'Sub-Saharan Africa', continents: ['AF'], exclude: northAfrica },
  { name: 'North Africa', include: northAfrica },
  { name: 'West Africa', aliases: ['Western Africa'], include: ['BF', 'BJ', 'CI', 'CV', 'GH', 'GM', 'GN', 'GW', 'LR', 'ML', 'MR', 'NE', 'NG', 'SL', 'SN', 'TG'] },
  { name: 'East Africa', aliases: ['Eastern Africa'], include: ['BI', 'DJ', 'ER', 'ET', 'KE', 'KM', 'MG', 'MU', 'MW', 'MZ', 'RW', 'SC', 'SO', 'SS', 'TZ', 'UG', 'ZM', 'ZW'] },
  { name: 'Southern Africa', include: ['BW', 'LS', 'NA', 'SZ', 'ZA'] },
  { name: 'Asia', continents: ['AS'] },
  { name: 'APAC', aliases: ['Asia Pacific', 'Asia-Pacific', 'Asia and Pacific'], continents: ['AS', 'OC'], exclude: middleEastCodes },
  { name: 'Southeast Asia', aliases: ['South East Asia', 'South-East Asia'], include: ['BN', 'ID', 'KH', 'LA', 'MM', 'MY', 'PH', 'SG', 'TH', 'TL', 'VN'] },
  { name: 'South Asia', include: ['AF', 'BD', 'BT', 'IN', 'LK', 'MV', 'NP', 'PK'] },
  { name: 'Oceania', continents: ['OC'] },
  { name: 'ANZ', aliases: ['Australasia'], include: ['AU', 'NZ'] },
  { name: 'North America', aliases: ['Northern America', 'NORAM', 'NAMER'], include: ['CA', 'MX', 'US'] },
  { name: 'Central America', include: centralAmerica },
  { name: 'South America', continents: ['SA'] },
  { name: 'LATAM', aliases: ['LatAm', 'Latin America'], continents: ['SA'], include: [...centralAmerica, 'CU', 'DO', 'HT', 'MX', 'PR'] },
  { name: 'Americas', aliases: ['The Americas', 'AMER'], continents: ['NA', 'SA'] },
  { name: 'DACH', include: ['AT', 'CH', 'DE'] },
  { name: 'Nordics', aliases: ['Nordic', 'Nordic countries'], include: ['DK', 'FI', 'IS', 'NO', 'SE'] },
  { name: 'Scandinavia', include: ['DK', 'NO', 'SE'] },
  { name: 'Benelux', include: ['BE', 'LU', 'NL'] },
  { name: 'Baltics', aliases: ['Baltic states'], include: ['EE', 'LT', 'LV'] },
  { name: 'CEE', aliases: ['Central and Eastern Europe', 'Eastern Europe', 'Central Eastern Europe'], include: ['AL', 'BA', 'BG', 'BY', 'CZ', 'EE', 'HR', 'HU', 'LT', 'LV', 'MD', 'ME', 'MK', 'PL', 'RO', 'RS', 'SI', 'SK', 'UA', 'XK'] },
];

interface CityEntry { name: string; countries: string[] }

interface Tables {
  byCode: Map<string, Country>;
  countryNames: Map<string, string>;
  regions: Map<string, Place>;
  compounds: string[];
  alsoSubdivision: Map<string, string>;
}

const regionCountries = (r: RegionDefinition, byCode: Map<string, Country>): string[] => {
  const set = new Set<string>();
  for (const c of countryList) if (r.continents?.includes(c.continent)) set.add(c.code);
  for (const code of r.include ?? []) if (byCode.has(code)) set.add(code);
  for (const code of r.exclude ?? []) set.delete(code);
  return [...set].sort();
};

// Every country and region name that a location would be split inside: "Bosnia and Herzegovina", "Taiwan, Province of China". Longest first.
const buildCompounds = (): string[] => {
  const names: string[] = [];
  const add = (name: string) => {
    if (/[,()&/]/.test(name) || name.toLowerCase().includes(' and ')) names.push(name);
  };
  for (const c of countryList) add(c.name);
  for (const aliases of Object.values(countryAliases)) aliases.forEach(add);
  for (const r of regionDefinitions) [r.name, ...(r.aliases ?? [])].forEach(add);
  return names.sort((a, b) => (a.length !== b.length ? b.length - a.length : a < b ? -1 : a > b ? 1 : 0));
};

let built: Tables | undefined;
const tables = (): Tables => {
  if (built) return built;
  const t: Tables = { byCode: new Map(), countryNames: new Map(), regions: new Map(), compounds: [], alsoSubdivision: new Map() };
  for (const c of countryList) {
    t.byCode.set(c.code, c);
    t.countryNames.set(fold(c.name), c.code);
  }
  for (const [code, names] of Object.entries(countryAliases)) for (const n of names) t.countryNames.set(fold(n), code);
  for (const [code, names] of Object.entries(subdivisions)) {
    for (const n of names) {
      if (t.countryNames.has(fold(n))) {
        t.alsoSubdivision.set(fold(n), code);
        continue;
      }
      t.countryNames.set(fold(n), code);
    }
  }
  for (const r of regionDefinitions) {
    const place: Place = { kind: 'region', name: r.name, countries: regionCountries(r, t.byCode), elsewhere: [] };
    for (const n of [r.name, ...(r.aliases ?? [])]) t.regions.set(fold(n), place);
  }
  t.compounds = buildCompounds();
  built = t;
  return t;
};

// The city table, as the lines of cities.tsv: folded name, comma-separated country codes (most populous first), display name.
let cityTable: Map<string, CityEntry> | undefined;
let cityTsv = '';

// Installs the city table. A name that is also a country's or a region's is left to it.
export function loadCities(tsv: string): void {
  cityTsv = tsv;
  cityTable = undefined;
}

const cities = (): Map<string, CityEntry> => {
  if (cityTable) return cityTable;
  const t = tables();
  const map = new Map<string, CityEntry>();
  for (const line of cityTsv.split('\n')) {
    const first = line.indexOf('\t');
    const second = first < 0 ? -1 : line.indexOf('\t', first + 1);
    if (second < 0) continue;
    const name = line.slice(0, first);
    if (t.countryNames.has(name) || t.regions.has(name)) continue;
    map.set(name, { name: line.slice(second + 1), countries: line.slice(first + 1, second).split(',') });
  }
  for (const [abbreviation, name] of Object.entries(cityAbbreviations)) {
    const city = map.get(fold(name));
    if (city) map.set(fold(abbreviation), city);
  }
  cityTable = map;
  return map;
};

const countryPlaceOf = (t: Tables, code: string): Place => ({ kind: 'country', name: t.byCode.get(code)!.name, countries: [code], elsewhere: [] });

// Resolves a place name to a region, a country or a city, in that order.
// Cities are looked up only when `withCities` is set: a city name alone is often a word ("Nice", "Mobile").
export function lookup(name: string, withCities: boolean): Place | undefined {
  const t = tables();
  const key = fold(name);
  const region = t.regions.get(key);
  if (region) return region;
  const code = t.countryNames.get(key);
  if (code) return countryPlaceOf(t, code);
  if (!withCities) return undefined;
  const city = cities().get(key);
  if (city) return { kind: 'city', name: city.name, countries: city.countries.slice(0, 1), elsewhere: city.countries.slice(1) };
  return undefined;
}

export const compounds = (): string[] => tables().compounds;

// The country in which a country's name is also a state's: "Georgia" is a state of the US.
export const subdivisionOf = (name: string): string | undefined => tables().alsoSubdivision.get(fold(name));

export function countryPlace(code: string): Place | undefined {
  const t = tables();
  return t.byCode.has(code) ? countryPlaceOf(t, code) : undefined;
}

export const countryByCode = (code: string): Country | undefined => tables().byCode.get(code.trim().toUpperCase());

// Reads a country as a person writes it, or by its ISO code, and returns its code.
export function resolveCountry(name: string): string | undefined {
  const key = fold(name);
  const code = tables().countryNames.get(key);
  if (code) return code;
  if (key.length === 2) return countryByCode(key)?.code;
  return undefined;
}

export const countryName = (code: string): string => countryByCode(code)?.name ?? code;

// The IANA zone's offset from UTC in hours without daylight saving, from a static table: no Intl, no clock.
const zoneIndex = new Map(timeZones.map((z) => [z.id.toLowerCase(), z]));
export const timeZoneByID = (id: string) => zoneIndex.get(id.trim().toLowerCase());
