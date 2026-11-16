// A port of the RemoteOK source (remoteok.go), ADR-0037.
// RemoteOK's terms ask consumers to link back and name it: every job keeps its RemoteOK URL and the id 'remoteok'.
// The feed leads with a legal notice that is not a job, and RemoteOK asks API users not to hammer it:
// the host keeps an answer for REMOTEOK_CACHE_MS.
import {
  describe,
  getJson,
  isObject,
  list,
  num,
  parseTime,
  sourceJob,
  str,
  strings,
  type DoFetch,
  type FullSourceJob,
} from './job-source.ts';

export const REMOTEOK_URL = 'https://remoteok.com/api';
export const REMOTEOK_CACHE_MS = 30 * 60 * 1000;

export function toJob(entry: unknown): FullSourceJob | undefined {
  // An entry is a job by having a position and a URL, not by where it sits: the notice, or one too incomplete to act on, is skipped.
  if (!isObject(entry) || str(entry.position) === '' || str(entry.url) === '') return undefined;
  const min = num(entry.salary_min);
  const max = num(entry.salary_max);
  const stated = min > 0 || max > 0; // zero is "not stated", not "unpaid"
  return sourceJob({
    sourceId: 'remoteok',
    sourceJobId: str(entry.id),
    canonicalUrl: str(entry.url),
    title: str(entry.position).trim(),
    companyName: str(entry.company).trim(), // the feed carries trailing spaces
    descriptionMarkdown: describe(str(entry.description)),
    locations: str(entry.location) === '' ? [] : [str(entry.location)],
    remoteType: 'remote',
    skills: strings(entry.tags),
    postedAt: parseTime(str(entry.date)),
    ...(stated
      ? { salaryMin: min, salaryMax: max, salaryCurrency: 'USD', salaryPeriod: 'yearly' }
      : {}),
  });
}

export async function fetchRemoteOK(
  doFetch: DoFetch,
  url = REMOTEOK_URL,
): Promise<FullSourceJob[]> {
  const body = await getJson(doFetch, url, 'remoteOK');
  if (!Array.isArray(body)) throw new Error('failed to decode the RemoteOK feed');
  return list(body)
    .map(toJob)
    .filter((j): j is FullSourceJob => j !== undefined);
}
