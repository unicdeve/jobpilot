// The scout run loop on the device (ADR-0037), ported from ScoutService.Run: collect jobs, pre-screen, score, decide, create
// applications for review up to the ceiling, and record the trail. Pure orchestration over injected ports.
// A scout never contacts an employer and never writes application content (ADR-0014, ADR-0034): the only ports that reach
// outside are `search` (the job-source APIs discovery already calls) and `score`.
import type { Db } from '../shared/db.ts';
import { eligible, preScreen, type EligibilityInput, type ScoutJob } from './scout.ts';
import { newApplication, type Tracked } from '../applications/tracking.ts';
import { markSavedSearchRun, getSavedSearch } from './saved-search.ts';
import {
  addEvent,
  finishRun,
  getScout,
  markScoutRun,
  startRun,
  type RunTrigger,
} from './scout-store.ts';

// Scoring costs one model call per job, so a run reads at most this many (a run can reject every job it looks at).
export const maxJobsConsideredPerRun = 200;

export type RunJob = ScoutJob & { id: string; companyName: string; url?: string };

export interface Verdict {
  jobId: string;
  jobTitle: string;
  considered: boolean;
  score: number;
  hasScore: boolean;
  eligible: boolean;
  reason: string;
}

export interface RunReport {
  scoutId: string;
  runId: string;
  considered: number;
  created: number;
  ceilingHit: boolean;
  verdicts: Verdict[];
  error: string;
}

// What a search of the job sources found, for the trail. A source that did not answer is its own step.
export interface SearchReport {
  jobs: number;
  new: number;
  sources: { name: string; status: 'ok' | 'failed' | 'unreachable' }[];
}

export interface RunPorts {
  db: Db;
  // The clock, read at the start, for each trail step and at the end. Storage modules never read it themselves.
  now(): number;
  // The id of the run record.
  newRunId(): string;
  // Only for a scout whose source is 'sources': search the job sources, which stores what they return.
  search?(criteria: import('../search/criteria.ts').SearchCriteria): Promise<SearchReport>;
  // Stored jobs, including those the user ruled out, so each is passed over with its reason in the trail.
  listJobs(limit: number): Promise<RunJob[]>;
  // The job ids the candidate already has an application for.
  appliedJobIds(): Promise<Set<string>>;
  // A match score, or undefined when none is available. A throw is a job that could not be scored.
  score(job: RunJob): Promise<number | undefined>;
  // Persists the reviewable application a scout produces. A throw ends the run, keeping what was created.
  createApplication(app: Tracked): Promise<void>;
  // Checked between jobs; true stops the run, keeping its work.
  cancelled?(): boolean;
}

// The application a scout makes for an eligible job: DISCOVERED, for the user to review. Nothing is written for the candidate.
export const reviewApplication = (job: RunJob, now: number): Tracked | undefined =>
  newApplication(
    { id: job.id, title: job.title, company: job.companyName, url: job.url ?? '' },
    now,
  );

const message = (e: unknown): string => (e instanceof Error ? e.message : String(e));

// Runs a scout once, because the user asked. A run that fails partway keeps the applications already created and records the
// failure (AC-006). `report` is null only when the scout could not be started at all (unknown or disabled).
export async function runScout(
  ports: RunPorts,
  scoutId: string,
  trigger: RunTrigger = 'manual',
): Promise<{ report: RunReport | null; error: string }> {
  const { db } = ports;
  const scout = await getScout(db, scoutId);
  if (scout === undefined)
    return { report: null, error: 'failed to load the scout: scout not found' };
  if (!scout.isEnabled) return { report: null, error: `scout "${scout.name}" is disabled` };

  // The run is recorded before anything that can fail, so a failure is a run like any other.
  const run = await startRun(db, { id: ports.newRunId(), scoutId, trigger }, ports.now());
  const report: RunReport = {
    scoutId,
    runId: run.id,
    considered: 0,
    created: 0,
    ceilingHit: false,
    verdicts: [],
    error: '',
  };
  let seq = 0;
  // Trail writes never fail a run: the real output is the applications.
  const save = async (
    action: string,
    status: string,
    inputSummary: string,
    outputSummary: string,
    policyResult = '',
  ): Promise<void> => {
    try {
      await addEvent(db, {
        runId: run.id,
        seq: seq++,
        action,
        status,
        inputSummary,
        outputSummary,
        policyResult,
        createdAt: ports.now(),
      });
    } catch {
      /* see above */
    }
  };
  const record = (a: string, s: string, i: string, o: string) => save(a, s, i, o);
  const decide = (a: string, policy: string, i: string, o: string) => save(a, 'ok', i, o, policy);

  let runErr = '';
  try {
    const search = await getSavedSearch(db, scout.savedSearchId);
    if (search === undefined) throw new Error("failed to load the scout's saved search: not found");
    await record(
      'scout_started',
      'ok',
      `scout "${scout.name}", search "${search.name}"`,
      `floor ${scout.minMatchScore}, ceiling ${scout.maxApplicationsPerRun}, source ${scout.source}`,
    );

    if (scout.source === 'sources') {
      if (ports.search === undefined)
        throw new Error('failed to search the job sources: no source search is available');
      let found: SearchReport;
      try {
        found = await ports.search(search.criteria);
      } catch (e) {
        throw new Error(`failed to search the job sources: ${message(e)}`);
      }
      // The search has now run, as it would have from Discover. Failing to record that costs one repeated search, not the run.
      try {
        await markSavedSearchRun(db, search.id, ports.now());
      } catch (e) {
        await record('search_run_not_recorded', 'error', search.name, message(e));
      }
      const unanswered = found.sources.filter((s) => s.status !== 'ok');
      for (const s of unanswered) {
        await record(
          s.status === 'unreachable' ? 'source_unreachable' : 'source_failed',
          'error',
          s.name,
          s.status === 'unreachable' ? 'could not be reached' : 'answered with an error',
        );
      }
      await record(
        'sources_searched',
        'ok',
        search.name,
        `${found.jobs} jobs returned, ${found.new} new; ${found.sources.length - unanswered.length} of ${found.sources.length} sources answered`,
      );
    }

    let jobs: RunJob[];
    try {
      jobs = await ports.listJobs(maxJobsConsideredPerRun);
    } catch (e) {
      throw new Error(`failed to read stored jobs: ${message(e)}`);
    }
    await record('jobs_collected', 'ok', scout.source, `${jobs.length} jobs to consider`);

    let applied: Set<string>;
    try {
      applied = new Set(await ports.appliedJobIds());
    } catch (e) {
      throw new Error(`failed to read existing applications: ${message(e)}`);
    }

    for (const job of jobs) {
      if (ports.cancelled?.()) throw new Error('the run was cancelled');
      // A run that has made its quota stops looking rather than scoring jobs it cannot act on (AC-005).
      if (report.created >= scout.maxApplicationsPerRun) {
        report.ceilingHit = true;
        await decide(
          'ceiling_reached',
          'denied',
          '',
          `stopped after creating ${report.created} applications`,
        );
        break;
      }
      report.considered++;
      const verdict: Verdict = {
        jobId: job.id,
        jobTitle: job.title,
        considered: true,
        score: 0,
        hasScore: false,
        eligible: false,
        reason: '',
      };
      const input: EligibilityInput = {
        job,
        criteria: search.criteria,
        score: 0,
        hasScore: false,
        alreadyApplied: applied.has(job.id),
        createdSoFar: report.created,
      };

      const [pass, why] = preScreen(scout, input);
      if (!pass) {
        verdict.reason = why;
        report.verdicts.push(verdict);
        await decide('job_rejected', 'denied', job.title, why);
        continue;
      }

      let score: number | undefined;
      try {
        score = await ports.score(job);
      } catch (e) {
        if (ports.cancelled?.()) {
          verdict.reason = 'the run was cancelled';
          report.verdicts.push(verdict);
          throw new Error('the run was cancelled');
        }
        // One job that cannot be scored does not end the run; the failure is recorded, not swallowed.
        verdict.reason = `could not be scored: ${message(e)}`;
        report.verdicts.push(verdict);
        await record('job_scoring_failed', 'error', job.title, message(e));
        continue;
      }
      if (ports.cancelled?.()) {
        verdict.reason = 'the run was cancelled';
        report.verdicts.push(verdict);
        throw new Error('the run was cancelled');
      }

      // An unavailable score is neither zero nor clearing the floor: Eligible says so.
      if (score !== undefined) {
        input.score = score;
        input.hasScore = true;
        verdict.score = score;
        verdict.hasScore = true;
      }
      const [ok, reason] = eligible(scout, input);
      verdict.eligible = ok;
      verdict.reason = reason;
      if (!ok) {
        report.verdicts.push(verdict);
        await decide('job_rejected', 'denied', job.title, reason);
        continue;
      }

      const app = reviewApplication(job, ports.now());
      try {
        if (app === undefined) throw new Error('the job has no id or title');
        await ports.createApplication(app);
      } catch (e) {
        verdict.eligible = false;
        verdict.reason = `could not create an application: ${message(e)}`;
        report.verdicts.push(verdict);
        await record('application_failed', 'error', job.title, message(e));
        throw new Error(`failed to create an application for "${job.title}": ${message(e)}`);
      }
      report.created++;
      applied.add(job.id);
      report.verdicts.push(verdict);
      await decide('application_created', 'allowed', job.title, `application ${app.id}, ${reason}`);
    }
  } catch (e) {
    runErr = message(e);
  }

  // However the run ends, the record and the scout's last-run time are closed off.
  const completed = ports.now();
  try {
    await finishRun(db, run.id, runErr === '' ? 'completed' : 'failed', runErr, completed);
  } catch {
    /* the report survives */
  }
  await record(
    'scout_finished',
    runErr === '' ? 'completed' : 'failed',
    `${report.considered} jobs considered`,
    `${report.created} applications created`,
  );
  try {
    await markScoutRun(db, scoutId, completed);
  } catch {
    /* the user can still see the run */
  }
  report.error = runErr;
  return { report, error: runErr };
}
