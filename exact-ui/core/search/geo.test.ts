import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { gunzipSync } from 'node:zlib';
import { citiesTsv } from './cities-data.ts';
import { fold, loadCities, lookup, placeIn, warmSteps } from './geo.ts';
import { loadBundledCities } from './geo-cities.ts';

// The cases are internal/core/domain/geo/geo_test.go's (TestLookup_Cities, TestLookup_ShortCityNames).

test('the generated table is the Go package table', () => {
  const gz = readFileSync(
    new URL('../../../internal/core/domain/geo/cities.tsv.gz', import.meta.url),
  );
  expect(citiesTsv).toBe(gunzipSync(gz).toString('utf8'));
});

test('until the table is loaded no city is a place', () => {
  loadCities('');
  expect(lookup('London', true)).toBeUndefined();
});

test('a city is found after the table is built in steps, as warmUp does', () => {
  loadBundledCities();
  for (const step of warmSteps) step();
  const cases: Record<string, string> = {
    London: 'GB',
    Berlin: 'DE',
    München: 'DE',
    Munich: 'DE',
    Zürich: 'CH',
    Zurich: 'CH',
    Paris: 'FR',
    Lagos: 'NG',
    'San Francisco': 'US',
    NYC: 'US',
    'New York': 'US',
    Toronto: 'CA',
    Lille: 'FR',
    Utrecht: 'NL',
    Seattle: 'US',
  };
  for (const [written, code] of Object.entries(cases)) {
    const place = lookup(written, true);
    expect(place?.kind, written).toBe('city');
    expect(place?.countries, written).toEqual([code]);
  }
  expect(lookup('London', false)).toBeUndefined();
  for (const word of ['Remote', 'Hybrid', 'Onsite', 'Worldwide', 'Anywhere', 'Global', 'Full-time'])
    expect(lookup(word, true), word).toBeUndefined();
  for (const code of ['Tor', 'ACT', 'SEA', 'LON']) expect(lookup(code, true), code).toBeUndefined();
  for (const [written, name] of Object.entries({
    NYC: 'New York City',
    SF: 'San Francisco',
    Ulm: 'Ulm',
  }))
    expect(lookup(written, true)?.name).toBe(name);
});

test('a lookup before the steps have run finishes the table itself', () => {
  loadBundledCities();
  expect(lookup('Lille', true)?.countries).toEqual(['FR']);
});

test('the table is read over many steps, and a lookup mid-way still completes it', () => {
  loadBundledCities();
  warmSteps[0]!();
  warmSteps[1]!();
  expect(lookup('Seattle', true)?.countries).toEqual(['US']);
  expect(warmSteps.length).toBeGreaterThan(10);
});

test('a city also in another country keeps the others, and placeIn picks one', () => {
  loadBundledCities();
  const london = lookup('London', true)!;
  expect(london.elsewhere).toContain('CA');
  expect(placeIn(london, 'CA')?.countries).toEqual(['CA']);
  expect(placeIn(london, 'JP')).toBeUndefined();
  expect(fold("Villeneuve-d'Ascq")).toBe('villeneuve d ascq');
});
