import { beforeEach, expect, test } from 'bun:test';
import type { Db } from '../shared/db.ts';
import { emptyCriteria } from '../search/criteria.ts';
import {
  availableName, createSavedSearch, defaultSavedSearch, deleteSavedSearch, ensureSavedSearchSchema, getSavedSearch, importTemplates,
  listSavedSearches, markSavedSearchRun, searchRefreshInterval, setDefaultSavedSearch, staleSavedSearches, updateSavedSearch,
} from './saved-search.ts';
import { ensureScoutSchema } from './scout-store.ts';
import { memoryDb } from '../shared/test-db.ts';

// The cases are the Go tests' (internal/core/services/savedsearch_test.go).
let db: Db;
let n = 0;
const id = () => `s${++n}`;
const go = { ...emptyCriteria, keywords: 'golang backend' };
const create = async (name: string, c = go, now = 1) => {
  const r = await createSavedSearch(db, id(), name, c, now);
  if (!r.ok) throw new Error(r.error);
  return r.value;
};
beforeEach(async () => { db = memoryDb(); n = 0; await ensureSavedSearchSchema(db); await ensureScoutSchema(db); });

test('the first search becomes the default, the second does not', async () => {
  expect((await create('Go roles')).isDefault).toBe(true);
  expect((await create('Design roles', { ...emptyCriteria, keywords: 'design' })).isDefault).toBe(false);
});
test('criteria that narrow nothing are rejected', async () => {
  const r = await createSavedSearch(db, id(), 'Everything', emptyCriteria, 1);
  expect(!r.ok && r.error).toContain('at least one criterion');
});
test('a search needs a name', async () => {
  const r = await createSavedSearch(db, id(), '   ', go, 1);
  expect(!r.ok && r.error).toContain('needs a name');
});
test('names are unique regardless of case', async () => {
  await create('Go roles');
  const r = await createSavedSearch(db, id(), 'go ROLES', go, 1);
  expect(!r.ok && r.error).toContain('already exists');
});
test('the name is trimmed', async () => { expect((await create('  Go roles  ')).name).toBe('Go roles'); });
test('work-from is upper-cased and must be a country', async () => {
  expect((await create('Mine', { ...go, workFrom: ' de ' })).criteria.workFrom).toBe('DE');
  const bad = await createSavedSearch(db, id(), 'Bad', { ...go, workFrom: 'ZZZ' }, 1);
  expect(bad.ok).toBe(false);
  const shift = await createSavedSearch(db, id(), 'Shift', { ...go, hoursShift: 13 }, 1);
  expect(!shift.ok && shift.error).toContain('hours shift');
});
test('criteria round-trip through storage', async () => {
  const c = { ...go, excludedCompanies: ['Acme'], salaryMin: 50000, fitHours: true, hoursShift: 3 };
  const made = await create('Full', c);
  expect((await getSavedSearch(db, made.id))!.criteria).toEqual({ ...c, workFrom: '' });
});

test('update: keeping its own name is not a clash, and default survives', async () => {
  const made = await create('Go roles');
  const a = await updateSavedSearch(db, made.id, 'Go roles', { ...emptyCriteria, keywords: 'rust' }, 5);
  expect(a.ok && a.value.criteria.keywords).toBe('rust');
  const b = await updateSavedSearch(db, made.id, 'Renamed', go, 6);
  expect(b.ok && b.value.isDefault).toBe(true);
  const c = await updateSavedSearch(db, made.id, 'Renamed', emptyCriteria, 7);
  expect(!c.ok && c.error).toContain('at least one criterion');
  expect((await updateSavedSearch(db, 'nope', 'X', go, 7)).ok).toBe(false);
});

test('set default demotes the previous holder, leaving exactly one', async () => {
  const first = await create('Go roles');
  const second = await create('Rust roles', { ...emptyCriteria, keywords: 'rust' });
  const r = await setDefaultSavedSearch(db, second.id, 9);
  expect(r.ok && r.value.isDefault).toBe(true);
  expect((await defaultSavedSearch(db))!.id).toBe(second.id);
  expect((await getSavedSearch(db, first.id))!.isDefault).toBe(false);
});

test('deleting the default promotes another; deleting the last leaves none', async () => {
  const first = await create('Go roles');
  await create('Rust roles', { ...emptyCriteria, keywords: 'rust' });
  await deleteSavedSearch(db, first.id, 9);
  expect((await defaultSavedSearch(db))!.name).toBe('Rust roles');
  const only = (await listSavedSearches(db))[0];
  await deleteSavedSearch(db, only.id, 10);
  expect(await defaultSavedSearch(db)).toBeUndefined();
});

test('deleting a search deletes its scouts and says how many', async () => {
  const s = await create('Go roles');
  await db.execute(`INSERT INTO scouts (id, name, saved_search_id, min_match_score, max_applications_per_run, source, created_at, updated_at) VALUES ('c1','C',?,70,5,'local',1,1)`, [s.id]);
  const r = await deleteSavedSearch(db, s.id, 2);
  expect(r.ok && r.value.scoutsDeleted).toBe(1);
  expect((await db.query('SELECT id FROM scouts')).rows).toHaveLength(0);
});

test('record run and stale', async () => {
  const backend = await create('Backend', { ...emptyCriteria, title: 'backend' });
  const frontend = await create('Frontend', { ...emptyCriteria, title: 'frontend' });
  const names = async (now: number) => (await staleSavedSearches(db, now)).map((s) => s.name);
  const now = Date.UTC(2026, 8, 29, 12);
  expect(await names(now)).toEqual(['Backend', 'Frontend']);
  await markSavedSearchRun(db, frontend.id, now);
  expect((await getSavedSearch(db, frontend.id))!.lastRunAt).toBe(now);
  expect(await names(now)).toEqual(['Backend']);
  expect(await names(now + searchRefreshInterval)).toEqual([backend.name, frontend.name]);
});

test('templates: each becomes an ordinary search, the first is the default', async () => {
  const r = await importTemplates(db, ['backend-engineer', 'fullstack-engineer'], '', 1, id);
  expect(r.ok).toBe(true);
  if (!r.ok) return;
  expect(r.value.map((s) => s.name)).toEqual(['Backend Engineer', 'Full-stack Engineer']);
  expect(r.value.map((s) => s.isDefault)).toEqual([true, false]);
  expect(await listSavedSearches(db)).toHaveLength(2);
});
test('templates: a second copy is numbered, a taken name is not overwritten', async () => {
  await create('backend engineer', { ...emptyCriteria, keywords: 'rust' });
  const a = await importTemplates(db, ['backend-engineer'], '', 1, id);
  const b = await importTemplates(db, ['backend-engineer'], '', 1, id);
  expect(a.ok && a.value[0].name).toBe('Backend Engineer 2');
  expect(b.ok && b.value[0].name).toBe('Backend Engineer 3');
});
test('templates: a repeated id is imported once, an unknown id writes nothing, nothing chosen is refused', async () => {
  const twice = await importTemplates(db, ['backend-engineer', 'backend-engineer'], '', 1, id);
  expect(twice.ok && twice.value).toHaveLength(1);
  const bad = await importTemplates(db, ['frontend-engineer', 'not-a-template'], '', 1, id);
  expect(!bad.ok && bad.error).toContain('not-a-template');
  expect(await listSavedSearches(db)).toHaveLength(1);
  const none = await importTemplates(db, [], '', 1, id);
  expect(!none.ok && none.error).toContain('no templates chosen');
});
test('availableName finds the lowest free number', () => {
  expect(availableName('A', new Set(['a', 'a 2']))).toBe('A 3');
});
