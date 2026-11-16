// A port of the Himalayas source (himalayas.go), ADR-0037.
// The board holds a hundred thousand jobs, so a fetch takes a bounded slice: four pages of 20 for the whole board,
// at most five search pages when the criteria carry words. Answers are kept an hour by the host (HIMALAYAS_CACHE_MS),
// per query string.
import type { SearchCriteria } from '../search/criteria.ts';
import { describe, encodeQuery, fromUnix, getJson, isObject, list, normalizeEmploymentType, num, sourceJob, str, strings, type DoFetch, type FullSourceJob } from './job-source.ts';

export const HIMALAYAS_URL = 'https://himalayas.app/jobs/api';
export const HIMALAYAS_CACHE_MS = 60 * 60 * 1000;
export const HIMALAYAS_PAGE_SIZE = 20;
export const HIMALAYAS_MAX_PAGES = 4;
export const HIMALAYAS_MAX_SEARCH_PAGES = 5;

type Asked = Pick<SearchCriteria, 'title' | 'keywords' | 'workFrom'>;
const words = (s: string): string => s.split(/\s+/).filter((w) => w !== '').join(' ');

// Only the words and the country are sent: the search endpoint takes them, and nothing else is trusted.
export function himalayasQuery(c: Asked): Record<string, string> {
  const query: Record<string, string> = {};
  const q = words(`${c.title} ${c.keywords}`);
  if (q !== '') {
    query.q = q;
    const country = (c.workFrom ?? '').trim();
    if (country !== '') query.country = country.toUpperCase();
  }
  return query;
}

export function toJob(item: unknown): FullSourceJob | undefined {
  // No title or no application link: not a job anyone can apply to.
  if (!isObject(item) || str(item.title) === '' || str(item.applicationLink) === '') return undefined;
  const restrictions = strings(item.locationRestrictions);
  let description = describe(str(item.description));
  if (description.trim() === '') description = str(item.excerpt);
  const min = num(item.minSalary);
  const max = num(item.maxSalary);
  return sourceJob({
    sourceId: 'himalayas', sourceJobId: str(item.guid), canonicalUrl: str(item.applicationLink), title: str(item.title).trim(),
    companyName: str(item.companyName).trim(), descriptionMarkdown: description,
    // Himalayas writes worldwide as no restriction, which would otherwise read as not stated.
    locations: restrictions.length === 0 ? ['Worldwide'] : restrictions, remoteType: 'remote',
    employmentType: normalizeEmploymentType(str(item.employmentType)), skills: strings(item.categories),
    postedAt: fromUnix(num(item.pubDate)), expiresAt: fromUnix(num(item.expiryDate)),
    ...(min > 0 || max > 0 ? { salaryMin: min, salaryMax: max, salaryCurrency: str(item.currency).toUpperCase(), salaryPeriod: str(item.salaryPeriod).toLowerCase() } : {}),
  });
}

async function fetchPage(doFetch: DoFetch, endpoint: string, base: Record<string, string>, page: number): Promise<FullSourceJob[]> {
  const query = { ...base };
  if (base.q !== undefined) query.page = String(page + 1);
  else { query.limit = String(HIMALAYAS_PAGE_SIZE); query.offset = String(page * HIMALAYAS_PAGE_SIZE); }
  const body = await getJson(doFetch, `${endpoint}?${encodeQuery(query)}`, 'himalayas');
  if (!isObject(body)) throw new Error('failed to decode the Himalayas response');
  return list(body.jobs).map(toJob).filter((j): j is FullSourceJob => j !== undefined);
}

export async function fetchHimalayas(doFetch: DoFetch, criteria: Asked, baseUrl = HIMALAYAS_URL): Promise<FullSourceJob[]> {
  const query = himalayasQuery(criteria);
  const searching = query.q !== undefined;
  const endpoint = searching ? `${baseUrl}/search` : baseUrl;
  const pages = searching ? HIMALAYAS_MAX_SEARCH_PAGES : HIMALAYAS_MAX_PAGES;
  const jobs: FullSourceJob[] = [];
  const seen = new Set<string>();
  for (let page = 0; page < pages; page++) {
    let batch: FullSourceJob[];
    try {
      batch = await fetchPage(doFetch, endpoint, query, page);
    } catch (e) {
      if (jobs.length > 0) break; // a later page failing must not discard the pages that worked
      throw e;
    }
    if (batch.length === 0) break;
    for (const job of batch) if (!seen.has(job.canonicalUrl)) { seen.add(job.canonicalUrl); jobs.push(job); }
  }
  return jobs;
}
