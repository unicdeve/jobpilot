import { describe, expect, test } from 'bun:test';
import { newScoutForm, loadScoutForm, saveScoutForm, searchChoices, describeCriteria, newScoutId } from './scout-editor.ts';
import { emptyCriteria } from '../search/criteria.ts';
import { ensureSavedSearchSchema, listSavedSearches } from './saved-search.ts';
import { ensureScoutSchema, listScouts } from './scout-store.ts';
import { memoryDb } from '../shared/test-db.ts';

async function fresh() { const db = memoryDb(); await ensureSavedSearchSchema(db); await ensureScoutSchema(db); return db; }
const typed = { ...newScoutForm, name: 'Go roles', keywords: ' golang  remote ' };

describe('saveScoutForm', () => {
  test('creates a scout with a saved search of its own', async () => {
    const db = await fresh();
    const saved = await saveScoutForm(db, newScoutId, typed, 1000);
    expect(saved.ok).toBe(true);
    const scouts = await listScouts(db);
    expect(scouts).toHaveLength(1);
    expect(scouts[0]!.maxApplicationsPerRun).toBe(10);
    const searches = await listSavedSearches(db);
    expect(searches[0]!.name).toBe('Go roles');
    expect(searches[0]!.criteria.keywords).toBe('golang remote');
  });

  test('a template becomes a search, and a saved search can be chosen', async () => {
    const db = await fresh();
    await saveScoutForm(db, newScoutId, { ...typed, choice: 'template:backend-engineer' }, 1);
    expect((await listSavedSearches(db))[0]!.criteria.title).toBe('back end');
    const second = await saveScoutForm(db, newScoutId, { ...newScoutForm, name: 'Again', choice: `search:search-1` }, 2);
    expect(second.ok).toBe(true);
    expect(await listSavedSearches(db)).toHaveLength(1);
  });

  test('refuses what the Go rules refuse, in a sentence', async () => {
    const db = await fresh();
    const bad = (f: Partial<typeof typed>) => saveScoutForm(db, newScoutId, { ...typed, ...f }, 1);
    expect(await bad({ name: ' ' })).toEqual({ ok: false, error: 'A scout needs a name.' });
    expect((await bad({ minScore: '101' })) as { error: string }).toMatchObject({ error: expect.stringContaining('between 0 and 100') });
    expect((await bad({ ceiling: '51' })) as { error: string }).toMatchObject({ error: expect.stringContaining('between 1 and 50') });
    expect((await bad({ minScore: 'abc' })).ok).toBe(false);
    expect((await bad({ keywords: ' ' })).ok).toBe(false);
    expect(await listScouts(db)).toHaveLength(0);
    expect(await listSavedSearches(db)).toHaveLength(0);
  });

  test('editing keeps the search when the criteria did not change, and loads back as typed', async () => {
    const db = await fresh();
    await saveScoutForm(db, newScoutId, typed, 1);
    const id = (await listScouts(db))[0]!.id;
    expect((await saveScoutForm(db, id, { ...typed, minScore: '80', enabled: false }, 2)).ok).toBe(true);
    expect(await listSavedSearches(db)).toHaveLength(1);
    expect(await loadScoutForm(db, id)).toEqual({ name: 'Go roles', choice: '', keywords: 'golang remote', minScore: '80', ceiling: '10', enabled: false });
    await saveScoutForm(db, id, { ...typed, keywords: 'rust' }, 3);
    expect(await listSavedSearches(db)).toHaveLength(2);
    expect((await listScouts(db))[0]!.savedSearchId).toBe('search-2');
  });
});

describe('helpers', () => {
  test('choices list typed, saved and template searches', async () => {
    const db = await fresh();
    await saveScoutForm(db, newScoutId, typed, 1);
    const choices = await searchChoices(db);
    expect(choices[0]!.value).toBe('');
    expect(choices.some((c) => c.value === 'search:search-1')).toBe(true);
    expect(choices.some((c) => c.value === 'template:go-engineer')).toBe(true);
  });
  test('describes criteria', () => {
    expect(describeCriteria({ ...emptyCriteria, title: 'back end', keywords: 'go' })).toBe('title back end · keywords go');
    expect(describeCriteria(emptyCriteria)).toBe('No criteria');
  });
});
