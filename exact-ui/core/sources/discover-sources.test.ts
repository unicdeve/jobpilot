import { describe, expect, test } from 'bun:test';
import { emptyCriteria } from '../search/criteria.ts';
import { realUrls, searchBoard, sourceUrls } from './discover-sources.ts';
import type { DoFetch } from './job-source.ts';
import { memoryDb } from '../shared/test-db.ts';

const json = (body: unknown, status = 200) => ({ ok: status < 400, status, json: async () => body, text: async () => JSON.stringify(body) });
const urls = sourceUrls('http://x/sources', 'http://x/remotive');
const remotive = { jobs: [{ id: 1, url: 'https://remotive.com/j/1', title: 'Go Developer', company_name: 'Acme', tags: [], job_type: 'full_time', publication_date: '2026-11-10T09:00:00', candidate_required_location: 'Worldwide', description: '<p>Go</p>' }] };
const remoteok = [{ legal: 'notice' }, { id: '9', position: 'Go Engineer', company: 'Globex', url: 'https://remoteok.com/9', description: 'Go', tags: [], date: '2026-11-11T09:00:00Z' }];

function fetcher(calls: string[]): DoFetch {
  return async (url) => {
    calls.push(url);
    if (url.startsWith(urls.remotive)) return json(remotive);
    if (url.startsWith(urls.remoteok)) return json(remoteok);
    if (url.startsWith(urls.arbeitnow)) return json({ data: [] });
    return json({}, 500);
  };
}

describe('sourceUrls', () => {
  test('real addresses by default, a stand-in per source under a base', () => {
    expect(sourceUrls().remoteok).toBe(realUrls.remoteok);
    expect(sourceUrls('http://x/s/').hn).toBe('http://x/s/hn');
    expect(sourceUrls('', 'http://r').remotive).toBe('http://r');
  });
});

describe('searchBoard', () => {
  test('one failing source does not fail the search, and is named', async () => {
    const found = await searchBoard(memoryDb(), fetcher([]), urls, 1000, { ...emptyCriteria, keywords: 'go' });
    expect(found.error).toBe('');
    expect(found.jobs.map((j) => `${j.sourceId}:${j.sourceJobId}`).sort()).toEqual(['remoteok:9', 'remotive:1']);
    const failed = found.sources.filter((s) => s.status !== 'ok').map((s) => s.sourceId).sort();
    expect(failed).toEqual(['hackernews', 'himalayas', 'jobicy', 'weworkremotely']);
  });

  test('a listing is kept for the source cache window and asked again after', async () => {
    const db = memoryDb(); const calls: string[] = [];
    const c = { ...emptyCriteria, keywords: 'go' };
    await searchBoard(db, fetcher(calls), urls, 0, c);
    const first = calls.filter((u) => u.startsWith(urls.remoteok)).length;
    await searchBoard(db, fetcher(calls), urls, 10 * 60 * 1000, c);
    expect(calls.filter((u) => u.startsWith(urls.remoteok)).length).toBe(first); // RemoteOK: 30 minutes
    await searchBoard(db, fetcher(calls), urls, 31 * 60 * 1000, c);
    expect(calls.filter((u) => u.startsWith(urls.remoteok)).length).toBe(first + 1);
    expect(calls.filter((u) => u.startsWith(urls.remotive)).length).toBe(1); // Remotive: 4 hours
  });

  test('a search with nothing to look for, or no answer, is an error to say', async () => {
    expect((await searchBoard(memoryDb(), fetcher([]), urls, 0, emptyCriteria)).error).toContain('at least one criterion');
    const none: DoFetch = async () => { throw new Error('offline'); };
    expect((await searchBoard(memoryDb(), none, urls, 0, { ...emptyCriteria, keywords: 'go' })).error).toContain('could not reach');
  });
});

describe('warmUp', () => {
  test('builds the lazy tables one turn at a time, once', async () => {
    const { warmUp } = await import('./discover-sources.ts');
    let turns = 0;
    await warmUp(async () => { turns++; });
    const first = turns;
    await warmUp(async () => { turns++; });
    expect(first).toBeGreaterThan(1);
    expect(turns).toBe(first);
  });
});
