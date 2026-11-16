import { expect, test } from 'bun:test';
import { emptyCriteria, type SearchCriteria } from '../search/criteria.ts';
import { createSavedSearch, ensureSavedSearchSchema, getSavedSearch } from './saved-search.ts';
import { runScout, type RunJob, type RunPorts } from './scout-run.ts';
import type { Scout } from './scout.ts';
import { createScout, ensureScoutSchema, getRun, getScout, listRunEvents } from './scout-store.ts';
import { memoryDb } from '../shared/test-db.ts';
import type { Tracked } from '../applications/tracking.ts';

// The cases are the Go tests' (internal/core/services/scout_test.go TestScoutRun_*).
const job = (id: string, title: string, extra: Partial<RunJob> = {}): RunJob => ({
  id, title, companyName: 'Acme', descriptionMarkdown: '', locations: [], remoteType: '', employmentType: '', salaryMin: 0, salaryMax: 0,
  url: `https://acme.test/${id}`, ...extra,
});

async function fixture(jobs: RunJob[], scores: Record<string, number | Error | undefined>, opts: {
  criteria?: SearchCriteria; scout?: Partial<Scout>; applied?: string[]; createFailsAfter?: number; search?: RunPorts['search'];
} = {}) {
  const db = memoryDb();
  await ensureSavedSearchSchema(db);
  await ensureScoutSchema(db);
  await createSavedSearch(db, 'search-1', 'Go roles', opts.criteria ?? { ...emptyCriteria, keywords: 'Go' }, 1);
  const made = await createScout(db, 'scout-1', {
    name: 'Daily Go', savedSearchId: 'search-1', minMatchScore: 70, maxApplicationsPerRun: 5, source: 'local', isEnabled: true,
    schedule: { kind: 'off', everyHours: 0, dailyAt: 0 }, ...opts.scout,
  }, 2);
  if (!made.ok) throw new Error(made.error);
  const created: Tracked[] = [];
  const scored: string[] = [];
  let clock = 1000;
  const ports: RunPorts = {
    db, now: () => clock++, newRunId: () => 'run-1',
    search: opts.search,
    listJobs: async () => jobs,
    appliedJobIds: async () => new Set(opts.applied ?? []),
    score: async (j) => {
      scored.push(j.id);
      const s = scores[j.id];
      if (s instanceof Error) throw s;
      return s;
    },
    createApplication: async (a) => {
      if (opts.createFailsAfter !== undefined && created.length >= opts.createFailsAfter) throw new Error('disk full');
      created.push(a);
    },
  };
  return { db, ports, created, scored };
}

test('only creates for eligible jobs, and does not score one the search excluded', async () => {
  const f = await fixture([job('job-1', 'Senior Go Engineer'), job('job-2', 'Go Engineer'), job('job-3', 'Frontend Engineer')],
    { 'job-1': 88, 'job-2': 55, 'job-3': 91 });
  const { report, error } = await runScout(f.ports, 'scout-1');
  expect(error).toBe('');
  expect(report!.created).toBe(1);
  expect(f.created.map((a) => a.jobId)).toEqual(['job-1']);
  const by = Object.fromEntries(report!.verdicts.map((v) => [v.jobId, v]));
  expect(by['job-2'].reason).toContain("below the scout's floor");
  expect(by['job-3'].reason).toContain('does not match the saved search');
  expect(f.scored).not.toContain('job-3');
});

test('what it creates is a DISCOVERED application for review, nothing submittable', async () => {
  const f = await fixture([job('job-1', 'Senior Go Engineer')], { 'job-1': 90 });
  await runScout(f.ports, 'scout-1');
  expect(f.created[0]).toMatchObject({ state: 'DISCOVERED', title: 'Senior Go Engineer', company: 'Acme' });
});

test('it records what it did', async () => {
  const f = await fixture([job('job-1', 'Senior Go Engineer'), job('job-2', 'Go Engineer')], { 'job-1': 88, 'job-2': 40 });
  const { report } = await runScout(f.ports, 'scout-1');
  const run = (await getRun(f.db, report!.runId))!;
  expect(run).toMatchObject({ status: 'completed', scoutId: 'scout-1', trigger: 'manual' });
  expect(run.completedAt).not.toBeNull();
  const events = await listRunEvents(f.db, report!.runId);
  const policy = (a: string) => events.find((e) => e.action === a)?.policyResult;
  expect(policy('job_rejected')).toBe('denied');
  expect(policy('application_created')).toBe('allowed');
  expect(policy('jobs_collected')).toBe('');
  expect(events.map((e) => e.action)).toEqual(['scout_started', 'jobs_collected', 'application_created', 'job_rejected', 'scout_finished']);
  expect(events.find((e) => e.action === 'job_rejected')!.outputSummary).toContain("below the scout's floor");
  expect((await getScout(f.db, 'scout-1'))!.lastRunAt).not.toBeNull();
});

test('it honours its ceiling and stops scoring', async () => {
  const jobs = ['A', 'B', 'C', 'D'].map((l, i) => job(`job-${i + 1}`, `Go Engineer ${l}`));
  const f = await fixture(jobs, { 'job-1': 90, 'job-2': 91, 'job-3': 92, 'job-4': 93 }, { scout: { maxApplicationsPerRun: 2 } });
  const { report } = await runScout(f.ports, 'scout-1');
  expect(report).toMatchObject({ created: 2, ceilingHit: true });
  expect(f.created).toHaveLength(2);
  expect(f.scored).toHaveLength(2);
});

test('a job that cannot be scored does not end the run, and the failure is recorded', async () => {
  const jobs = ['A', 'B', 'C'].map((l, i) => job(`job-${i + 1}`, `Go Engineer ${l}`));
  const f = await fixture(jobs, { 'job-1': 90, 'job-2': new Error('the model is unreachable'), 'job-3': 92 });
  const { report, error } = await runScout(f.ports, 'scout-1');
  expect(error).toBe('');
  expect(report!.created).toBe(2);
  const failed = (await listRunEvents(f.db, report!.runId)).find((e) => e.action === 'job_scoring_failed')!;
  expect(failed.status).toBe('error');
});

test('a score that is unavailable is rejected, not treated as zero', async () => {
  const f = await fixture([job('job-1', 'Go Engineer')], { 'job-1': undefined });
  const { report } = await runScout(f.ports, 'scout-1');
  expect(report!.created).toBe(0);
  expect(report!.verdicts[0].reason).toContain('could not be scored');
});

test('a failure partway keeps the applications already created', async () => {
  const f = await fixture([job('job-1', 'Go Engineer A'), job('job-2', 'Go Engineer B')], { 'job-1': 90, 'job-2': 91 }, { createFailsAfter: 1 });
  const { report, error } = await runScout(f.ports, 'scout-1');
  expect(error).toContain('failed to create an application');
  expect(report!.created).toBe(1);
  expect(report!.error).toBe(error);
  expect(f.created).toHaveLength(1);
  expect(await getRun(f.db, report!.runId)).toMatchObject({ status: 'failed' });
  expect((await getRun(f.db, report!.runId))!.error).not.toBe('');
});

test('jobs already applied to are skipped without being scored', async () => {
  const f = await fixture([job('job-1', 'Go Engineer')], { 'job-1': 95 }, { applied: ['job-1'] });
  const { report } = await runScout(f.ports, 'scout-1');
  expect(report!.created).toBe(0);
  expect(report!.verdicts).toHaveLength(1);
  expect(report!.verdicts[0].reason).toContain('already exists');
  expect(f.scored).toEqual([]);
});

test('a job the user ruled out is passed over with its reason, never scored', async () => {
  const f = await fixture([job('job-1', 'Senior Go Engineer'), job('job-2', 'Go Engineer', { notInterestedAt: '2026-11-01' })], { 'job-1': 90, 'job-2': 99 });
  const { report } = await runScout(f.ports, 'scout-1');
  expect(report!.created).toBe(1);
  expect(f.scored).not.toContain('job-2');
  const rejected = (await listRunEvents(f.db, report!.runId)).filter((e) => e.action === 'job_rejected').map((e) => e.outputSummary);
  expect(rejected).toContain('you marked this job not interested');
});

test('it refuses when disabled or unknown', async () => {
  const f = await fixture([], {}, { scout: { isEnabled: false } });
  expect((await runScout(f.ports, 'scout-1')).error).toContain('disabled');
  expect((await runScout(f.ports, 'nope')).report).toBeNull();
});

test('cancellation keeps partial work and finalizes the run', async () => {
  const f = await fixture([job('job-1', 'Go Engineer A'), job('job-2', 'Go Engineer B')], { 'job-1': 90, 'job-2': 91 });
  f.ports.cancelled = () => f.created.length >= 1;
  const { report, error } = await runScout(f.ports, 'scout-1');
  expect(error).toContain('cancelled');
  expect(report!.created).toBe(1);
  expect(await getRun(f.db, report!.runId)).toMatchObject({ status: 'failed' });
  expect((await getScout(f.db, 'scout-1'))!.lastRunAt).not.toBeNull();
});

test('a sources scout searches first, records the search and each source that did not answer', async () => {
  const f = await fixture([job('job-1', 'Go Engineer')], { 'job-1': 90 }, {
    scout: { source: 'sources' },
    search: async () => ({ jobs: 3, new: 2, sources: [{ name: 'Remotive', status: 'ok' }, { name: 'Workable', status: 'unreachable' }] }),
  });
  const { report } = await runScout(f.ports, 'scout-1');
  const events = await listRunEvents(f.db, report!.runId);
  expect(events.map((e) => e.action)).toContain('source_unreachable');
  expect(events.find((e) => e.action === 'sources_searched')!.outputSummary).toBe('3 jobs returned, 2 new; 1 of 2 sources answered');
  expect((await getSavedSearch(f.db, 'search-1'))!.lastRunAt).not.toBeNull();
});

test('a sources scout with no way to search fails the run, keeping the record', async () => {
  const f = await fixture([], {}, { scout: { source: 'sources' } });
  const { report, error } = await runScout(f.ports, 'scout-1');
  expect(error).toContain('failed to search the job sources');
  expect(await getRun(f.db, report!.runId)).toMatchObject({ status: 'failed' });
});

test('a saved search deleted under a scout fails the run, not the app', async () => {
  const f = await fixture([], {});
  await f.db.execute("UPDATE scouts SET saved_search_id = 'gone'");
  expect((await runScout(f.ports, 'scout-1')).error).toContain("saved search");
});
