// A port of the Jobicy source (jobicy.go), ADR-0037.
// Jobicy's terms: a job keeps its Jobicy URL (canonicalUrl) and a feed is asked at most once an hour (JOBICY_CACHE_MS, kept by the host
// per `jobicyFeedUrl`), the locations list a day (JOBICY_LOCATIONS_MS).
// With no country the whole board is one feed; with one, the "anywhere" feed plus each location Jobicy lists that covers the country.
import { describe, encodeQuery, getJson, isObject, list, normalizeEmploymentType, num, parseTime, sourceJob, str, strings, type DoFetch, type FullSourceJob } from './job-source.ts';
import * as geo from '../search/geo.ts';

export const JOBICY_URL = 'https://jobicy.com/api/v2/remote-jobs';
export const JOBICY_CACHE_MS = 60 * 60 * 1000;
export const JOBICY_LOCATIONS_MS = 24 * 60 * 60 * 1000;
export const JOBICY_COUNT = 200;
export const JOBICY_ANYWHERE = 'anywhere';

export interface JobicyLocation { name: string; slug: string }

export function toJob(item: unknown): FullSourceJob | undefined {
  if (!isObject(item) || str(item.jobTitle) === '' || str(item.url) === '') return undefined;
  let description = describe(str(item.jobDescription));
  if (description.trim() === '') description = str(item.jobExcerpt);
  const type = strings(item.jobType);
  const min = num(item.salaryMin);
  const max = num(item.salaryMax);
  return sourceJob({
    sourceId: 'jobicy', sourceJobId: String(item.id ?? ''), canonicalUrl: str(item.url), title: str(item.jobTitle).trim(),
    companyName: str(item.companyName).trim(), descriptionMarkdown: description,
    locations: str(item.jobGeo).split(',').map((p) => p.trim()).filter((p) => p !== ''), remoteType: 'remote',
    employmentType: type.length > 0 ? normalizeEmploymentType(type[0]!) : '', skills: strings(item.jobIndustry), postedAt: parseTime(str(item.pubDate)),
    ...(min > 0 || max > 0 ? { salaryMin: min, salaryMax: max, salaryCurrency: str(item.salaryCurrency).toUpperCase(), salaryPeriod: str(item.salaryPeriod).toLowerCase() } : {}),
  });
}

export const jobicyFeedUrl = (geoSlug: string, baseUrl = JOBICY_URL): string =>
  `${baseUrl}?${encodeQuery({ count: String(JOBICY_COUNT), ...(geoSlug !== '' ? { geo: geoSlug } : {}) })}`;

export async function fetchJobicyLocations(doFetch: DoFetch, baseUrl = JOBICY_URL): Promise<JobicyLocation[]> {
  const body = await getJson(doFetch, `${baseUrl}?${encodeQuery({ get: 'locations' })}`, 'jobicy');
  if (!isObject(body)) throw new Error('failed to decode the Jobicy response');
  return list(body.locations).filter(isObject).map((l) => ({ name: str(l.geoName), slug: str(l.geoSlug) }));
}

// The feeds a search for `workFrom` reads, in order: '' is the whole board.
export function jobicyFeeds(workFrom: string, locations: JobicyLocation[]): string[] {
  const country = workFrom.trim().toUpperCase();
  if (country === '') return [''];
  const feeds = [JOBICY_ANYWHERE];
  for (const location of locations) {
    if (location.slug === JOBICY_ANYWHERE) continue;
    const place = geo.lookup(location.name, false);
    if (place !== undefined && place.countries.includes(country)) feeds.push(location.slug);
  }
  return feeds;
}

async function fetchFeed(doFetch: DoFetch, geoSlug: string, baseUrl: string): Promise<FullSourceJob[]> {
  const body = await getJson(doFetch, jobicyFeedUrl(geoSlug, baseUrl), 'jobicy');
  if (!isObject(body)) throw new Error('failed to decode the Jobicy response');
  return list(body.jobs).map(toJob).filter((j): j is FullSourceJob => j !== undefined);
}

// `locations` may be passed when the host kept them; otherwise they are read, only when a country asks for them.
export async function fetchJobicy(doFetch: DoFetch, workFrom = '', baseUrl = JOBICY_URL, locations?: JobicyLocation[]): Promise<FullSourceJob[]> {
  const known = workFrom.trim() === '' ? [] : (locations ?? await fetchJobicyLocations(doFetch, baseUrl));
  const jobs: FullSourceJob[] = [];
  const seen = new Set<string>();
  for (const slug of jobicyFeeds(workFrom, known)) {
    let batch: FullSourceJob[];
    try {
      batch = await fetchFeed(doFetch, slug, baseUrl);
    } catch (e) {
      if (jobs.length > 0) break;
      throw e;
    }
    for (const job of batch) if (!seen.has(job.canonicalUrl)) { seen.add(job.canonicalUrl); jobs.push(job); }
  }
  return jobs;
}
