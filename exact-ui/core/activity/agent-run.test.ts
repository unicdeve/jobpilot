import { describe, expect, test } from 'bun:test';
import { memoryDb } from '../shared/test-db.ts';
import {
  appendEvent,
  clearActivity,
  ensureAgentRunTables,
  failInterruptedRuns,
  finishRun,
  getRun,
  listRuns,
  startRun,
} from './agent-run-store.ts';
import { validAgentRunStatus, inProgress } from './agent-run.ts';
import { deletionRules, rulesReferencing, targetTakesId, validDeletionTarget } from './deletion.ts';

const setup = async () => {
  const db = memoryDb();
  await ensureAgentRunTables(db);
  return db;
};
const ev = { action: 'match', tool: 'scorer', status: 'ok', durationMs: 5 };

describe('agent runs', () => {
  test('record, finish once, list newest first', async () => {
    const db = await setup();
    await startRun(db, { id: 'a', scoutId: 's1', trigger: 'manual' }, 100);
    await startRun(db, { id: 'b', trigger: 'schedule' }, 200);
    expect((await appendEvent(db, 'a', ev, 110)).id).toBe('a:1');
    await appendEvent(
      db,
      'a',
      { ...ev, policyResult: 'denied', outputSummary: 'below floor' },
      120,
    );
    expect(await finishRun(db, 'a', 'completed', '', 130)).toBe(true);
    expect(await finishRun(db, 'a', 'failed', 'x', 140)).toBe(false);
    expect(await finishRun(db, 'nope', 'failed', 'x', 140)).toBe(false);
    const page = await listRuns(db);
    expect(page.total).toBe(2);
    expect(page.runs.map((r) => r.id)).toEqual(['b', 'a']);
    const d = (await getRun(db, 'a'))!;
    expect(d.run.status).toBe('completed');
    expect(d.run.completedAt).toBe(130);
    expect(d.run.eventCount).toBe(2);
    expect(d.events.map((e) => e.id)).toEqual(['a:1', 'a:2']);
    expect(d.events[1].policyResult).toBe('denied');
    expect(await getRun(db, 'zz')).toBeUndefined();
  });
  test('filter, page, interrupted, clear', async () => {
    const db = await setup();
    for (let i = 0; i < 3; i++) await startRun(db, { id: `r${i}` }, i);
    await finishRun(db, 'r0', 'failed', 'boom', 10);
    expect((await listRuns(db, { status: 'failed' })).total).toBe(1);
    expect((await getRun(db, 'r0'))!.run.error).toBe('boom');
    const p = await listRuns(db, { limit: 1, offset: 1 });
    expect(p.runs.map((r) => r.id)).toEqual(['r1']);
    expect(p.total).toBe(3);
    await failInterruptedRuns(db, 50);
    expect((await listRuns(db, { status: 'running' })).total).toBe(0);
    await clearActivity(db);
    expect((await listRuns(db)).total).toBe(0);
  });
  test('scout name when the scout exists', async () => {
    const db = await setup();
    await db.execute('CREATE TABLE scouts (id TEXT PRIMARY KEY, name TEXT)');
    await db.execute("INSERT INTO scouts VALUES ('s1', 'Go roles')");
    await startRun(db, { id: 'a', scoutId: 's1' }, 1);
    await startRun(db, { id: 'b', scoutId: 'gone' }, 2);
    const r = (await listRuns(db)).runs;
    expect(r.find((x) => x.id === 'a')!.scoutName).toBe('Go roles');
    expect(r.find((x) => x.id === 'b')!.scoutName).toBeUndefined();
  });
  test('statuses and deletion rules', () => {
    expect(validAgentRunStatus('done')).toBe(false);
    expect(inProgress('running')).toBe(true);
    expect(rulesReferencing('agent_run').map((r) => r.referencing)).toEqual([
      'agent_step',
      'notification',
    ]);
    expect(deletionRules.find((r) => r.referencing === 'agent_run')!.effect).toBe('detach');
    expect(validDeletionTarget('activity')).toBe(true);
    expect(targetTakesId('job') && !targetTakesId('everything')).toBe(true);
  });
});
