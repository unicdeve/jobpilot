import { describe, expect, test } from 'bun:test';
import { ensureApplicationSchema } from '../applications/application-store.ts';
import {
  ensureAgentRunTables,
  getRun as getAgentRun,
  listRuns as listAgentRuns,
} from '../activity/agent-run-store.ts';
import type { BoardSearch } from '../sources/discover-sources.ts';
import { sourceJob } from '../sources/job-source.ts';
import { readSignals } from '../search/signals.ts';
import { ensureSavedSearchSchema } from './saved-search.ts';
import { newScoutForm, newScoutId, saveScoutForm } from './scout-editor.ts';
import { runScoutNow } from './scout-manual.ts';
import { ensureScoutSchema, listRuns, listScouts } from './scout-store.ts';
import { memoryDb } from '../shared/test-db.ts';

async function setup() {
  const db = memoryDb();
  for (const f of [
    ensureApplicationSchema,
    ensureSavedSearchSchema,
    ensureScoutSchema,
    ensureAgentRunTables,
  ])
    await f(db);
  await saveScoutForm(db, newScoutId, { ...newScoutForm, name: 'Go', keywords: 'go' }, 1);
  return { db, scoutId: (await listScouts(db))[0]!.id };
}
const job = (id: string) => {
  const j = sourceJob({
    sourceId: 'remotive',
    sourceJobId: id,
    canonicalUrl: `https://r/${id}`,
    title: 'Go Developer',
    companyName: 'Acme',
    descriptionMarkdown: 'go',
  });
  return { ...j, ...readSignals(j) };
};
const board = (n: number): BoardSearch => ({
  jobs: Array.from({ length: n }, (_, i) => job(String(i + 1))),
  sources: [{ sourceId: 'remotive', sourceName: 'Remotive', status: 'ok', found: n, failing: [] }],
  error: '',
});

describe('runScoutNow', () => {
  test('with no model, jobs are not scored and no application is created', async () => {
    const { db, scoutId } = await setup();
    const result = await runScoutNow(
      { withDb: (w) => w(db), search: async () => board(2), now: 5000 },
      scoutId,
    );
    expect(result.ok).toBe(true);
    expect(result.message).toContain('2 jobs could not be scored because no model is set up');
    expect(result.message).toContain('0 applications created');
    expect(Number((await db.query('SELECT COUNT(*) FROM applications')).rows[0]![0])).toBe(0);
    expect((await listRuns(db, 5))[0]!.status).toBe('completed');
    expect((await listScouts(db))[0]!.lastRunAt).toBe(5000);
  });

  test('is recorded for Activity with its steps', async () => {
    const { db, scoutId } = await setup();
    await runScoutNow({ withDb: (w) => w(db), search: async () => board(1), now: 5000 }, scoutId);
    const page = await listAgentRuns(db);
    expect(page.total).toBe(1);
    expect(page.runs[0]).toMatchObject({
      taskType: 'scout_run',
      status: 'completed',
      scoutName: 'Go',
      trigger: 'manual',
    });
    const detail = await getAgentRun(db, page.runs[0]!.id);
    expect(detail!.events.map((e) => e.action)).toContain('job_rejected');
  });

  test('a scored job above the floor becomes an application for review', async () => {
    const { db, scoutId } = await setup();
    const result = await runScoutNow(
      { withDb: (w) => w(db), search: async () => board(1), now: 5000, score: async () => 90 },
      scoutId,
    );
    expect(result.message).toContain('1 application created');
    expect((await db.query('SELECT id, state FROM applications')).rows).toEqual([
      ['local:remotive:1', 'DISCOVERED'],
    ]);
  });

  test('a search no source answered fails the run, and says why', async () => {
    const { db, scoutId } = await setup();
    const failed: BoardSearch = { jobs: [], sources: [], error: 'no job source answered' };
    const result = await runScoutNow(
      { withDb: (w) => w(db), search: async () => failed, now: 5000 },
      scoutId,
    );
    expect(result.ok).toBe(false);
    expect(result.error).toContain('no job source answered');
    expect((await listRuns(db, 5))[0]!.status).toBe('failed');
  });

  test('a disabled or missing scout does not run', async () => {
    const { db, scoutId } = await setup();
    expect(
      (await runScoutNow({ withDb: (w) => w(db), search: async () => board(0), now: 1 }, 'nope'))
        .ok,
    ).toBe(false);
    await db.execute('UPDATE scouts SET is_enabled = 0');
    expect(
      (await runScoutNow({ withDb: (w) => w(db), search: async () => board(0), now: 1 }, scoutId))
        .error,
    ).toContain('Turn the scout on');
  });
});
