// What a scout run leaves for Activity (ADR-0037): the run and its steps, written through the agent-run recorder, and the one
// sentence the Scouts screen says after a run. Takes a Db and never reads the clock.
import type { Db } from '../shared/db.ts';
import { agentTaskScoutRun } from '../activity/agent-run.ts';
import { appendEvent, finishRun, startRun } from '../activity/agent-run-store.ts';
import type { RunReport } from './scout-run.ts';
import type { RunEvent, ScoutRun } from './scout-store.ts';

// Mirrors a finished scout run into the agent-run tables, step for step, so Activity lists it beside every other run.
export async function recordAgentRun(db: Db, run: ScoutRun, events: RunEvent[]): Promise<void> {
  await startRun(db, { id: run.id, scoutId: run.scoutId, trigger: run.trigger === 'schedule' ? 'schedule' : 'manual', taskType: agentTaskScoutRun }, run.startedAt);
  for (const e of events) {
    await appendEvent(db, run.id, { action: e.action, tool: 'scout', status: e.status, inputSummary: e.inputSummary, outputSummary: e.outputSummary,
      policyResult: e.policyResult, durationMs: 0 }, e.createdAt);
  }
  if (run.status !== 'running') await finishRun(db, run.id, run.status, run.error, run.completedAt ?? run.startedAt);
}

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`;

// A job that was never scored never becomes an application (the Go rule): say how many, and why.
export const unscoredCount = (report: RunReport): number => report.verdicts.filter((v) => !v.hasScore && v.reason.includes('could not be scored')).length;

export function runSummary(report: RunReport, modelAvailable: boolean): string {
  if (report.error !== '') return `The run stopped: ${report.error}`;
  const base = `${plural(report.considered, 'job', 'jobs')} considered, ${plural(report.created, 'application', 'applications')} created.`;
  const unscored = unscoredCount(report);
  if (unscored === 0) return base;
  const why = modelAvailable ? 'the model gave no score' : 'no model is set up to score them';
  return `${base} ${plural(unscored, 'job', 'jobs')} could not be scored because ${why}, so none of those became an application.`;
}
