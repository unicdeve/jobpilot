// companywatch_test.go: watching a company, the suggestions, and the Company pages source, against a fake fetch and a bun:sqlite Db.
import { expect, test } from 'bun:test';
import { memoryDb } from '../shared/test-db.ts';
import type { BoardEndpoints, BoardFetch } from '../sources/company-boards.ts';
import { isUnreachable, PartialAnswerError } from '../sources/job-source.ts';
import { emptyCriteria } from '../search/criteria.ts';
import { ensureWatchSchema, listCompanies, renameCompany, deleteCompany } from './watch-store.ts';
import {
  companyForJob,
  companyPagesSource,
  suggestions,
  watch,
  watchJob,
  JobNotOnBoardError,
  type Watcher,
} from './watch.ts';
import { UnsupportedAddressError } from './company.ts';

const endpoints: BoardEndpoints = {
  greenhouseApi: 'http://x/gh',
  greenhouseEuApi: 'http://x/gh-eu',
  leverApi: 'http://x/lever',
  leverEuApi: 'http://x/lever-eu',
  ashbyApi: 'http://x/ashby',
  workableApi: 'http://x/workable',
  smartRecruitersApi: 'http://x/sr',
  leverPages: 'http://x/lp',
  leverEuPages: 'http://x/lep',
  ashbyPages: 'http://x/ap',
  smartRecruitersPages: 'http://x/srp',
};
const posting = (board: string, i: number, title: string) => ({
  id: i,
  title,
  absolute_url: `https://job-boards.greenhouse.io/${board}/jobs/${i}`,
  location: { name: 'Remote, Worldwide' },
  content: '',
  updated_at: '2026-09-01T00:00:00Z',
});

// Each greenhouse board answers by name: a list of titles, 'gone' (404) or 'down' (no answer).
async function setup(
  boards: Record<string, string[] | 'gone' | 'down'>,
  names: Record<string, string> = {},
) {
  const db = memoryDb();
  await ensureWatchSchema(db);
  let active = 0,
    most = 0;
  const fetch: BoardFetch = async (url) => {
    const m = /^http:\/\/x\/gh\/v1\/boards\/([^/?]+)(\/jobs)?/.exec(url);
    if (!m) throw new Error('connection refused');
    const board = m[1]!;
    active++;
    most = Math.max(most, active);
    await new Promise((r) => setTimeout(r, 5));
    active--;
    const answer = boards[board];
    if (answer === 'down' || answer === undefined) throw new Error('connection refused');
    if (answer === 'gone')
      return { ok: false, status: 404, json: async () => ({}), text: async () => '' };
    const body = m[2]
      ? { jobs: answer.map((t, i) => posting(board, i, t)) }
      : { name: names[board] ?? '' };
    return {
      ok: true,
      status: 200,
      json: async () => body,
      text: async () => JSON.stringify(body),
    };
  };
  let n = 0;
  const w: Watcher = { db, fetch, endpoints, newId: () => `company-${++n}` };
  return {
    w,
    db,
    most: () => most,
    reset: () => {
      most = 0;
    },
  };
}
const gh = (name: string) => `boards.greenhouse.io/${name}`;

test('watching reads the board once, names the company, and records the check', async () => {
  const { w } = await setup({ gitlab: ['AI Engineer', 'Sales'] }, { gitlab: 'GitLab' });
  const { company, created } = await watch(w, gh('gitlab'), 1000);
  expect(created).toBe(true);
  expect(company).toMatchObject({
    name: 'GitLab',
    check: 'ok',
    openRoles: 2,
    checkedAt: 1000,
    url: 'https://job-boards.greenhouse.io/gitlab',
    addedFrom: gh('gitlab'),
  });
  const again = await watch(w, 'https://job-boards.greenhouse.io/GitLab/jobs/1', 2000);
  expect(again.created).toBe(false);
  expect(again.company.id).toBe(company.id);
});

test('a board with no name from its provider is named from its address; a mistyped one is refused, not watched', async () => {
  const { w, db } = await setup({ 'cockroach-labs': [], gone: 'gone' });
  expect((await watch(w, gh('cockroach-labs'), 1)).company.name).toBe('Cockroach labs');
  await expect(watch(w, gh('gone'), 1)).rejects.toThrow('could not read');
  await expect(watch(w, 'https://gitlab.com/jobs', 1)).rejects.toThrow(UnsupportedAddressError);
  expect(await listCompanies(db)).toHaveLength(1);
});

test("watching a job's company reads its posting address, and a job off every board is refused", async () => {
  const { w, db } = await setup({ acme: ['Engineer'] }, { acme: 'Acme' });
  const job = {
    canonicalUrl: 'https://job-boards.greenhouse.io/acme/jobs/1',
    descriptionMarkdown: '',
  };
  expect((await companyForJob(db, job)).company).toBeUndefined();
  expect((await watchJob(w, job, 1)).company.name).toBe('Acme');
  expect((await companyForJob(db, job)).company?.name).toBe('Acme');
  await expect(
    watchJob(w, { canonicalUrl: 'https://remoteok.com/1', descriptionMarkdown: '' }, 1),
  ).rejects.toThrow(JobNotOnBoardError);
});

test('rename, unwatch and suggestions', async () => {
  const { w, db } = await setup({ gitlab: [] }, { gitlab: 'GitLab' });
  const { company } = await watch(w, gh('gitlab'), 1);
  expect((await suggestions(db)).some((s) => s.board === 'gitlab')).toBe(false);
  await renameCompany(db, company.id, 'My GitLab');
  expect((await listCompanies(db))[0]!.name).toBe('My GitLab');
  await deleteCompany(db, company.id);
  expect(await listCompanies(db)).toHaveLength(0);
  expect((await suggestions(db)).some((s) => s.board === 'gitlab')).toBe(true);
});

test('the source reads every company, names each posting, and records each check', async () => {
  const { w, db } = await setup(
    { gitlab: ['AI Engineer'], acme: ['Backend Engineer', 'Sales'] },
    { gitlab: 'GitLab', acme: 'Acme' },
  );
  await watch(w, gh('gitlab'), 1);
  await watch(w, gh('acme'), 1);
  const jobs = await companyPagesSource(w, 5000).search({ ...emptyCriteria, title: 'engineer' });
  expect(jobs).toHaveLength(3);
  expect(jobs.map((j) => j.companyName).sort()).toEqual(['Acme', 'Acme', 'GitLab']);
  for (const c of await listCompanies(db))
    expect(c).toMatchObject({ check: 'ok', checkedAt: 5000 });
});

test('no company watched is no jobs', async () => {
  const { w } = await setup({});
  expect(await companyPagesSource(w, 1).search({ ...emptyCriteria, title: 'engineer' })).toEqual(
    [],
  );
});

test('boards are read four at a time', async () => {
  const names = Array.from({ length: 10 }, (_, i) => `company${i}`);
  const { w, most, reset } = await setup(Object.fromEntries(names.map((n) => [n, []])));
  for (const n of names) await watch(w, gh(n), 1);
  reset();
  await companyPagesSource(w, 2).search({ ...emptyCriteria, title: 'engineer' });
  expect(most()).toBe(4);
});

test('a company that fails is named, the others are kept, and it stays on the list', async () => {
  const { w, db } = await setup(
    { gitlab: ['AI Engineer'], closed: [], down: [] },
    { gitlab: 'GitLab', closed: 'Closed Co', down: 'Down Co' },
  );
  for (const n of ['gitlab', 'closed', 'down']) await watch(w, gh(n), 1);
  // The boards change after they were watched.
  const first = w.fetch;
  const boards: Record<string, string> = { closed: 'gone', down: 'down' };
  w.fetch = async (url, init) => {
    const m = /\/boards\/([^/?]+)\/jobs/.exec(url);
    if (m && boards[m[1]!] === 'down') throw new Error('connection refused');
    if (m && boards[m[1]!] === 'gone')
      return { ok: false, status: 404, json: async () => ({}), text: async () => '' };
    return first(url, init);
  };
  let error: unknown;
  try {
    await companyPagesSource(w, 9).search({ ...emptyCriteria, title: 'engineer' });
  } catch (e) {
    error = e;
  }
  expect(error).toBeInstanceOf(PartialAnswerError);
  const partial = error as PartialAnswerError;
  expect(partial.failing).toEqual(['Closed Co', 'Down Co']);
  expect(partial.error).toBeUndefined();
  expect(partial.jobs).toHaveLength(1);
  const checks = Object.fromEntries((await listCompanies(db)).map((c) => [c.name, c.check]));
  expect(checks).toEqual({ 'Closed Co': 'gone', 'Down Co': 'unreachable', GitLab: 'ok' });
});

test('every company unreachable is the source unreachable; otherwise it failed', async () => {
  const { w } = await setup({ a: [], b: [] });
  await watch(w, gh('a'), 1);
  await watch(w, gh('b'), 1);
  const real = w.fetch;
  w.fetch = async () => {
    throw new Error('offline');
  };
  let error: unknown;
  try {
    await companyPagesSource(w, 2).search({ ...emptyCriteria, title: 'x' });
  } catch (e) {
    error = e;
  }
  expect(isUnreachable((error as PartialAnswerError).error)).toBe(true);
  w.fetch = async (url, init) =>
    url.includes('/boards/a/')
      ? { ok: false, status: 404, json: async () => ({}), text: async () => '' }
      : (() => {
          throw new Error('offline');
        })();
  error = undefined;
  try {
    await companyPagesSource(w, 2).search({ ...emptyCriteria, title: 'x' });
  } catch (e) {
    error = e;
  }
  expect((error as PartialAnswerError).error).toBeDefined();
  expect(isUnreachable((error as PartialAnswerError).error)).toBe(false);
  void real;
});
