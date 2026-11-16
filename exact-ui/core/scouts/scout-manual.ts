// "Run now" on the device (ADR-0037 section 3: a scout runs when the user asks). The job sources are searched first, with no
// database held, because a search can take a while; then the run itself is one serialized turn on the database.
// A scout never contacts an employer and never writes application content (ADR-0014, ADR-0034).
import type { SearchCriteria } from '../search/criteria.ts';
import type { Db } from '../shared/db.ts';
import type { BoardSearch } from '../sources/discover-sources.ts';
import { foundId } from '../sources/discover-sources.ts';
import { getTracked, saveTracked, trackedJobIds } from '../applications/application-store.ts';
import { getSavedSearch } from './saved-search.ts';
import { recordAgentRun, runSummary } from './scout-activity.ts';
import { runScout, type RunJob } from './scout-run.ts';
import { getRun, getScout, listRunEvents } from './scout-store.ts';

export interface ManualRunEnv {
  // One turn on the database: the app serializes these, so two never overlap.
  withDb<T>(work: (db: Db) => Promise<T>): Promise<T>;
  search(criteria: SearchCriteria): Promise<BoardSearch>;
  // The clock, read by the caller: a data source cannot read it.
  now: number;
  // The model that scores a job, or undefined while none is chosen (ADR-0037 section 4). Never invented here.
  score?(job: RunJob): Promise<number | undefined>;
}
export interface ManualRunResult {
  ok: boolean;
  error: string;
  message: string;
}

export async function runScoutNow(env: ManualRunEnv, scoutId: string): Promise<ManualRunResult> {
  const prepared = await env.withDb(async (db) => {
    const scout = await getScout(db, scoutId);
    return {
      scout,
      search: scout === undefined ? undefined : await getSavedSearch(db, scout.savedSearchId),
    };
  });
  if (prepared.scout === undefined)
    return { ok: false, error: 'This scout is no longer stored.', message: '' };
  if (!prepared.scout.isEnabled)
    return { ok: false, error: 'Turn the scout on before running it.', message: '' };

  let board: BoardSearch = { jobs: [], sources: [], error: '' };
  if (prepared.scout.source === 'sources' && prepared.search !== undefined)
    board = await env.search(prepared.search.criteria);

  return env.withDb(async (db) => {
    const { report, error } = await runScout(
      {
        db,
        now: () => env.now,
        newRunId: () => `${scoutId}:${env.now}`,
        search: async () => {
          if (board.error !== '') throw new Error(board.error);
          return {
            jobs: board.jobs.length,
            new: board.jobs.length,
            sources: board.sources.map((s) => ({ name: s.sourceName, status: s.status })),
          };
        },
        listJobs: async (limit) =>
          board.jobs
            .slice(0, limit)
            .map((j): RunJob => ({ ...j, id: foundId(j), url: j.canonicalUrl })),
        appliedJobIds: () => trackedJobIds(db),
        score: env.score ?? (async () => undefined),
        createApplication: async (app) => {
          if ((await getTracked(db, app.id)) === undefined) await saveTracked(db, app);
        },
      },
      scoutId,
      'manual',
    );
    if (report === null)
      return { ok: false, error: error.replace(/^failed to load the scout: /, ''), message: '' };
    // The trail is for Activity: failing to mirror it must not hide what the run did.
    try {
      const run = await getRun(db, report.runId);
      if (run !== undefined) await recordAgentRun(db, run, await listRunEvents(db, report.runId));
    } catch {
      /* the run is still in the scout's own history */
    }
    return { ok: error === '', error, message: runSummary(report, env.score !== undefined) };
  });
}
