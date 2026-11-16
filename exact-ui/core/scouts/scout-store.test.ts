import { beforeEach, expect, test } from 'bun:test';
import type { Db } from '../shared/db.ts';
import { emptyCriteria } from '../search/criteria.ts';
import { createSavedSearch, ensureSavedSearchSchema } from './saved-search.ts';
import type { Scout } from './scout.ts';
import {
  addEvent,
  createScout,
  deleteScout,
  ensureScoutSchema,
  finishRun,
  getRun,
  getScout,
  listRunEvents,
  listRuns,
  listScouts,
  markScoutRun,
  startRun,
  updateScout,
} from './scout-store.ts';
import { memoryDb } from '../shared/test-db.ts';

// The cases are the Go tests' (services/scout_test.go TestScoutService_CreateScout, and UpdateScout's rules).
let db: Db;
const off = { kind: 'off', everyHours: 0, dailyAt: 0 };
const base = (adjust: Partial<Scout> = {}): Scout => ({
  name: 'New',
  savedSearchId: 'search-1',
  minMatchScore: 70,
  maxApplicationsPerRun: 5,
  source: 'local',
  isEnabled: true,
  schedule: off,
  ...adjust,
});
beforeEach(async () => {
  db = memoryDb();
  await ensureSavedSearchSchema(db);
  await ensureScoutSchema(db);
  await createSavedSearch(db, 'search-1', 'Go', { ...emptyCriteria, keywords: 'Go' }, 1);
});

test('create fills in the defaults a caller may omit', async () => {
  const r = await createScout(db, 'c1', base({ source: '', maxApplicationsPerRun: 0 }), 10);
  expect(r.ok && r.value).toMatchObject({
    source: 'local',
    maxApplicationsPerRun: 10,
    scheduleSetAt: null,
    lastRunAt: null,
  });
  expect(await getScout(db, 'c1')).toEqual(r.ok ? r.value : undefined);
});
test('create refuses a scout whose saved search does not exist', async () => {
  const r = await createScout(db, 'c1', base({ savedSearchId: 'nope' }), 10);
  expect(!r.ok && r.error).toContain('saved search could not be found');
});
test('create refuses an invalid scout', async () => {
  const r = await createScout(db, 'c1', base({ name: '  ' }), 10);
  expect(!r.ok && r.error).toContain('needs a name');
  const s = await createScout(
    db,
    'c2',
    base({ schedule: { kind: 'every', everyHours: 25, dailyAt: 0 } }),
    10,
  );
  expect(!s.ok && s.error).toContain('every 1 to 24 hours');
});
test('a scheduled scout records when its schedule was set; the schedule is stored normalized', async () => {
  const r = await createScout(
    db,
    'c1',
    base({ schedule: { kind: 'every', everyHours: 6, dailyAt: 99 } }),
    10,
  );
  expect(r.ok && r.value.scheduleSetAt).toBe(10);
  expect((await getScout(db, 'c1'))!.schedule).toEqual({
    kind: 'every',
    everyHours: 6,
    dailyAt: 0,
  });
});
test('update carries over last run and created, and moves scheduleSetAt only when the schedule changes', async () => {
  await createScout(db, 'c1', base({ schedule: { kind: 'every', everyHours: 6, dailyAt: 0 } }), 10);
  await markScoutRun(db, 'c1', 50);
  const same = await updateScout(
    db,
    'c1',
    base({ name: 'Renamed', schedule: { kind: 'every', everyHours: 6, dailyAt: 0 } }),
    100,
  );
  expect(same.ok && same.value).toMatchObject({
    name: 'Renamed',
    scheduleSetAt: 10,
    lastRunAt: 50,
    createdAt: 10,
    updatedAt: 100,
  });
  const changed = await updateScout(
    db,
    'c1',
    base({ schedule: { kind: 'daily', everyHours: 0, dailyAt: 480 } }),
    200,
  );
  expect(changed.ok && changed.value.scheduleSetAt).toBe(200);
  const cleared = await updateScout(db, 'c1', base(), 300);
  expect(cleared.ok && cleared.value.scheduleSetAt).toBe(null);
  expect((await updateScout(db, 'nope', base(), 1)).ok).toBe(false);
});
test('delete removes the scout with its runs and trail', async () => {
  await createScout(db, 'c1', base(), 10);
  await startRun(db, { id: 'r1', scoutId: 'c1', trigger: 'manual' }, 11);
  await addEvent(db, {
    runId: 'r1',
    seq: 0,
    action: 'scout_started',
    status: 'ok',
    inputSummary: '',
    outputSummary: '',
    policyResult: '',
    createdAt: 11,
  });
  expect((await deleteScout(db, 'c1')).ok).toBe(true);
  expect(await listScouts(db)).toEqual([]);
  expect(await listRuns(db, 10)).toEqual([]);
  expect(await listRunEvents(db, 'r1')).toEqual([]);
  expect((await deleteScout(db, 'c1')).ok).toBe(false);
});
test('runs list newest first and events in order', async () => {
  await createScout(db, 'c1', base(), 10);
  await startRun(db, { id: 'r1', scoutId: 'c1', trigger: 'manual' }, 11);
  await startRun(db, { id: 'r2', scoutId: 'c1', trigger: 'manual' }, 12);
  await finishRun(db, 'r1', 'failed', 'boom', 13);
  expect((await listRuns(db, 10)).map((r) => r.id)).toEqual(['r2', 'r1']);
  expect(await getRun(db, 'r1')).toMatchObject({
    status: 'failed',
    error: 'boom',
    completedAt: 13,
  });
  for (const seq of [1, 0])
    await addEvent(db, {
      runId: 'r1',
      seq,
      action: `a${seq}`,
      status: 'ok',
      inputSummary: '',
      outputSummary: '',
      policyResult: '',
      createdAt: 1,
    });
  expect((await listRunEvents(db, 'r1')).map((e) => e.action)).toEqual(['a0', 'a1']);
});
