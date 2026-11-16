// The job sources the app searches and the on-device cache in front of them (ADR-0037), the host half of Go's discovery wiring.
// Each source keeps its own terms: a listing is kept for the source's *_CACHE_MS (Remotive's own window is REMOTIVE_CACHE_MS) and
// asked for again after. A data source cannot read the clock, so `now` is passed in. Nothing here writes anything for the candidate.
import type { SearchCriteria } from '../search/criteria.ts';
import type { Db } from '../shared/db.ts';
import { DiscoveryError, dedupe, searchSources, type DiscoverySource, type FoundJob, type SourceOutcome } from './discovery.ts';
import { warmSteps } from '../search/workable.ts';
import { SourceUnreachableError, sourceJob, type DoFetch, type FullSourceJob } from './job-source.ts';
import { fetchRemotive, REMOTIVE_URL } from './remotive.ts';
import { ARBEITNOW_CACHE_MS, ARBEITNOW_URL, fetchArbeitnow } from './arbeitnow.ts';
import { fetchHackerNews, HN_CACHE_MS, HN_URL } from './hacker-news.ts';
import { fetchHimalayas, himalayasQuery, HIMALAYAS_CACHE_MS, HIMALAYAS_URL } from './himalayas.ts';
import { fetchJobicy, JOBICY_CACHE_MS, JOBICY_URL } from './jobicy.ts';
import { fetchRemoteOK, REMOTEOK_CACHE_MS, REMOTEOK_URL } from './remoteok.ts';
import { fetchWeWorkRemotely, WWR_CACHE_MS, WWR_URL } from './we-work-remotely.ts';

// Remotive's terms ask for few requests a day: the listing is kept this long.
export const REMOTIVE_CACHE_MS = 4 * 60 * 60 * 1000;

export interface SourceUrls { remotive: string; remoteok: string; arbeitnow: string; himalayas: string; jobicy: string; wwr: string; hn: string }
export const realUrls: SourceUrls = {
  remotive: REMOTIVE_URL, remoteok: REMOTEOK_URL, arbeitnow: ARBEITNOW_URL, himalayas: HIMALAYAS_URL, jobicy: JOBICY_URL, wwr: WWR_URL, hn: HN_URL,
};
// The address of every source, or a stand-in for each under `base` (for test and verify); `remotive` overrides Remotive alone.
export function sourceUrls(base = '', remotive = ''): SourceUrls {
  const root = base.replace(/\/$/, '');
  const at = (name: string, real: string) => (root === '' ? real : `${root}/${name}`);
  return {
    remotive: remotive !== '' ? remotive : at('remotive', realUrls.remotive), remoteok: at('remoteok', realUrls.remoteok),
    arbeitnow: at('arbeitnow', realUrls.arbeitnow), himalayas: at('himalayas', realUrls.himalayas), jobicy: at('jobicy', realUrls.jobicy),
    wwr: at('wwr', realUrls.wwr), hn: at('hn', realUrls.hn),
  };
}

export const cacheSchema = 'CREATE TABLE IF NOT EXISTS source_cache (key TEXT PRIMARY KEY, at INTEGER NOT NULL, body TEXT NOT NULL)';

// Wraps a fetch so a source's listing is kept in SQLite for `ttl`. A failure is never kept, so the next search asks again.
export function cachedSource(
  db: Db, now: number, spec: { id: string; name: string; ttl: number; key(c: SearchCriteria): string; fetchJobs(c: SearchCriteria): Promise<FullSourceJob[]> },
  lock: { turn: Promise<unknown> },
): DiscoverySource {
  // One statement at a time on the shared connection.
  const serial = <T>(work: () => Promise<T>): Promise<T> => { const run = lock.turn.then(work, work); lock.turn = run.catch(() => undefined); return run; };
  return {
    id: spec.id, name: spec.name, canSearch: true, filtersServerSide: false,
    async search(criteria) {
      const key = spec.key(criteria);
      const row = (await serial(() => db.query('SELECT at, body FROM source_cache WHERE key = ?', [key]))).rows[0];
      if (row !== undefined) {
        const at = Number(row[0]);
        if (now >= at && now - at < spec.ttl) { try { return JSON.parse(String(row[1])) as FullSourceJob[]; } catch { /* asked again below */ } }
      }
      const jobs = await spec.fetchJobs(criteria);
      try { await serial(() => db.execute('INSERT OR REPLACE INTO source_cache (key, at, body) VALUES (?, ?, ?)', [key, now, JSON.stringify(jobs)])); } catch { /* a listing not kept is only asked for again */ }
      return jobs;
    },
  };
}

export function discoverySources(db: Db, doFetch: DoFetch, urls: SourceUrls, now: number): DiscoverySource[] {
  const lock = { turn: Promise.resolve() as Promise<unknown> };
  const one = (id: string, name: string, ttl: number, fetchJobs: (c: SearchCriteria) => Promise<FullSourceJob[]>, key: (c: SearchCriteria) => string = () => id) =>
    cachedSource(db, now, { id, name, ttl, key, fetchJobs }, lock);
  return [
    one('remotive', 'Remotive', REMOTIVE_CACHE_MS, async () => (await fetchRemotive(async (u) => { try { return await doFetch(u); } catch (e) { throw new SourceUnreachableError('remotive', e); } }, urls.remotive)).map((j) => sourceJob(j))),
    one('remoteok', 'RemoteOK', REMOTEOK_CACHE_MS, () => fetchRemoteOK(doFetch, urls.remoteok)),
    one('arbeitnow', 'Arbeitnow', ARBEITNOW_CACHE_MS, () => fetchArbeitnow(doFetch, urls.arbeitnow)),
    one('himalayas', 'Himalayas', HIMALAYAS_CACHE_MS, (c) => fetchHimalayas(doFetch, c, urls.himalayas), (c) => `himalayas:${JSON.stringify(himalayasQuery(c))}`),
    one('jobicy', 'Jobicy', JOBICY_CACHE_MS, (c) => fetchJobicy(doFetch, c.workFrom ?? '', urls.jobicy), (c) => `jobicy:${(c.workFrom ?? '').toUpperCase()}`),
    one('weworkremotely', 'We Work Remotely', WWR_CACHE_MS, () => fetchWeWorkRemotely(doFetch, urls.wwr)),
    one('hackernews', 'Hacker News', HN_CACHE_MS, () => fetchHackerNews(doFetch, urls.hn)),
  ];
}

// The id a job is known by across sources, and the one an application for it is tracked under.
export const foundId = (job: { sourceId: string; sourceJobId: string }): string => `${job.sourceId}:${job.sourceJobId}`;

let warmed = false;
// Builds the tables a first read of a job's signals needs, one turn each, so none is one long turn. Done once.
export async function warmUp(turn: () => Promise<void>): Promise<void> {
  if (warmed) return;
  for (const step of warmSteps) { await turn(); step(); }
  warmed = true;
}

export interface BoardSearch { jobs: FoundJob[]; sources: SourceOutcome[]; error: string }

// Searches every source, isolating each one's failure, and dedupes by URL. A search no source answered, or one with nothing to
// look for, is an `error` for the view to say, never a throw: only a bug throws.
export async function searchBoard(db: Db, doFetch: DoFetch, urls: SourceUrls, now: number, criteria: SearchCriteria, turn?: () => Promise<void>): Promise<BoardSearch> {
  if (turn !== undefined) await warmUp(turn);
  await db.execute(cacheSchema);
  try {
    const found = await searchSources(discoverySources(db, doFetch, urls, now), criteria, undefined, turn);
    return { jobs: dedupe(found.jobs, new Map()).fresh, sources: found.sources, error: '' };
  } catch (e) {
    if (e instanceof DiscoveryError) return { jobs: [], sources: [], error: e.message };
    throw e;
  }
}
