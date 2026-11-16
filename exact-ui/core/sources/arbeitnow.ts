// A port of the Arbeitnow source (internal/adapters/secondary/discovery/arbeitnow.go), ADR-0037.
// Arbeitnow acts on none of its query parameters, so the listing is returned whole and discovery filters it.
// "Jobs are updated every hour": the host may keep an answer for ARBEITNOW_CACHE_MS and not ask again before.
import {
  describe,
  getJson,
  isObject,
  list,
  num,
  sourceJob,
  str,
  strings,
  type DoFetch,
  type FullSourceJob,
} from './job-source.ts';

export const ARBEITNOW_URL = 'https://www.arbeitnow.com/api/job-board-api';
export const ARBEITNOW_CACHE_MS = 15 * 60 * 1000;

export function toJob(item: unknown): FullSourceJob | undefined {
  if (!isObject(item)) return undefined;
  const created = num(item.created_at);
  return sourceJob({
    sourceId: 'arbeitnow',
    sourceJobId: str(item.slug),
    canonicalUrl: str(item.url),
    title: str(item.title),
    companyName: str(item.company_name),
    descriptionMarkdown: describe(str(item.description)),
    locations: str(item.location)
      .split(';')
      .map((l) => l.trim())
      .filter((l) => l !== ''),
    remoteType: item.remote === true ? 'remote' : 'onsite',
    employmentType: strings(item.job_types).join(', '),
    skills: strings(item.tags),
    postedAt: new Date(created * 1000).toISOString(),
  });
}

export async function fetchArbeitnow(
  doFetch: DoFetch,
  url = ARBEITNOW_URL,
): Promise<FullSourceJob[]> {
  const body = await getJson(doFetch, url, 'arbeitnow');
  if (!isObject(body) || !Array.isArray(body.data))
    throw new Error('failed to decode arbeitnow response');
  return list(body.data)
    .map(toJob)
    .filter((j): j is FullSourceJob => j !== undefined);
}
