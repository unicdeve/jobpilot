// The aggregation rules of Go's JobDiscoveryService.Search (internal/core/services/discovery_test.go), over injected sources.
import { expect, test } from 'bun:test';
import { emptyCriteria, type SearchCriteria } from '../search/criteria.ts';
import { dedupe, DiscoveryError, noAnswerError, normalizeWorkFrom, searchSources, searchable, type DiscoverySource } from './discovery.ts';
import { PartialAnswerError, SourceUnreachableError, sourceJob, type FullSourceJob } from './job-source.ts';

const job = (id: string, title: string, url = `https://x/${id}`): FullSourceJob =>
  sourceJob({ sourceId: 's', sourceJobId: id, canonicalUrl: url, title, companyName: 'Acme', descriptionMarkdown: `${title} role.`, locations: ['Berlin'], remoteType: 'remote' });
const source = (id: string, search: () => Promise<FullSourceJob[]>, over: Partial<DiscoverySource> = {}): DiscoverySource =>
  ({ id, name: id.toUpperCase(), canSearch: true, filtersServerSide: false, search, ...over });
const criteria = (over: Partial<SearchCriteria>): SearchCriteria => ({ ...emptyCriteria, ...over });
const code = async (p: Promise<unknown>) => ((await p.catch((e) => e)) as DiscoveryError).code;

test('empty criteria are refused, as is no searchable source', async () => {
  expect(await code(searchSources([source('a', async () => [])], emptyCriteria))).toBe('empty-criteria');
  expect(await code(searchSources([source('a', async () => [], { canSearch: false })], criteria({ title: 'go' })))).toBe('no-searchable-source');
  expect(searchable([source('a', async () => []), source('b', async () => [], { canSearch: false })]).map((s) => s.id)).toEqual(['a']);
});

test('a country must be one JobPilot knows', async () => {
  expect(normalizeWorkFrom(' ng ')).toBe('NG');
  expect(normalizeWorkFrom('')).toBe('');
  expect(await code(searchSources([source('a', async () => [])], criteria({ title: 'go', workFrom: 'ZZ' })))).toBe('invalid-work-from');
});

test('a source that cannot filter is filtered here, and one that can is trusted', async () => {
  const jobs = [job('1', 'Go Engineer'), job('2', 'Designer')];
  const r = await searchSources([source('a', async () => jobs), source('b', async () => [job('3', 'Chef')], { filtersServerSide: true })], criteria({ title: 'engineer' }));
  expect(r.jobs.map((j) => j.sourceJobId)).toEqual(['1', '3']);
  expect(r.sources.map((o) => [o.sourceId, o.status, o.found])).toEqual([['a', 'ok', 1], ['b', 'ok', 1]]);
  expect(r.jobs[0]!.signalsVersion).toBeGreaterThan(0); // signals are read before filtering
});

test('a failing source does not fail the search, and is not silent', async () => {
  const r = await searchSources([
    source('a', async () => { throw new Error('status 500'); }),
    source('b', async () => { throw new SourceUnreachableError('b', new Error('offline')); }),
    source('c', async () => [job('1', 'Go Engineer')]),
    source('d', async () => { throw 'a connector that panics'; }),
  ], criteria({ title: 'engineer' }));
  expect(r.jobs).toHaveLength(1);
  expect(r.sources.map((o) => o.status)).toEqual(['failed', 'unreachable', 'ok', 'failed']);
});

test('a source made of parts names the parts that did not answer', async () => {
  const partial = source('a', async () => { throw new PartialAnswerError(['gone-co']); });
  const r = await searchSources([partial, source('b', async () => [])], criteria({ title: 'go' }));
  expect(r.sources[0]).toMatchObject({ status: 'ok', failing: ['gone-co'] });
  const failed = source('a', async () => { throw new PartialAnswerError(['x'], new SourceUnreachableError('a', new Error('offline'))); });
  expect(await code(searchSources([failed], criteria({ title: 'go' })))).toBe('sources-unreachable');
});

test('no source answering is unreachable when none could be reached, else names each failure', async () => {
  const offline = () => source('a', async () => { throw new SourceUnreachableError('a', new Error('offline')); });
  expect(await code(searchSources([offline(), { ...offline(), id: 'b' }], criteria({ title: 'go' })))).toBe('sources-unreachable');
  const err = (await searchSources([offline(), source('b', async () => { throw new Error('500'); })], criteria({ title: 'go' })).catch((e) => e)) as DiscoveryError;
  expect(err.code).toBe('all-sources-failed');
  expect(err.message).toContain('A could not be reached');
  expect(err.message).toContain('B answered with an error');
  expect(noAnswerError([{ sourceId: 'a', sourceName: 'A', status: 'unreachable', found: 0, failing: [] }]).code).toBe('sources-unreachable');
});

test('a source answering with no jobs is an answer', async () => {
  const r = await searchSources([source('a', async () => [])], criteria({ title: 'go' }));
  expect(r.jobs).toEqual([]);
});

test('dedupe: the first source keeps a posting, a known one is not new, one ruled out is left out', async () => {
  const r = await searchSources([
    source('a', async () => [job('1', 'Go Engineer', 'https://x/same'), job('2', 'Go Dev', 'https://x/known'), job('3', 'Go Ops', 'https://x/ruled-out')]),
    source('b', async () => [job('9', 'Go Engineer', 'https://x/same'), job('4', 'Go SRE', 'https://x/new')]),
  ], criteria({ title: 'go' }));
  const known = new Map([['https://x/known', { notInterested: false }], ['https://x/ruled-out', { notInterested: true }]]);
  const { fresh, existingUrls } = dedupe(r.jobs, known);
  expect(fresh.map((j) => j.sourceJobId)).toEqual(['1', '4']);
  expect(existingUrls).toEqual(['https://x/known']);
});
