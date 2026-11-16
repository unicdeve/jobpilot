// Installs the generated city table (cities-data.ts, from the Go package's cities.tsv.gz) into geo.ts.
// Kept apart from geo.ts so a test or host that brings its own table does not carry the 1.4 MB string.
import { citiesTsv } from './cities-data.ts';
import { loadCities } from './geo.ts';

// Hands the table to geo.ts. Cheap: the table is read later, in slices, by warmUp (sources/discover-sources.ts) or on the first city lookup.
export function loadBundledCities(): void {
  loadCities(citiesTsv);
}
