// Wording for the agent trail, shared by the Activity screens and the
// per-scout views that link to them.

import type { AgentRunSummary, AgentTaskType } from "./types";

export const TASK_LABELS: Record<AgentTaskType, string> = {
  scout_run: "Scout run",
};

function taskLabel(taskType: string): string {
  return TASK_LABELS[taskType as AgentTaskType] ?? taskType;
}

/// runKind is what kind of run it was, and for a scout run whether its
/// schedule started it rather than the user.
export function runKind(run: Pick<AgentRunSummary, "task_type" | "trigger">): string {
  const label = taskLabel(run.task_type);
  return run.trigger === "schedule" ? `${label}, on schedule` : label;
}

/// runSubject names what a run served: its scout, which can be deleted
/// after the run, which keeps its record regardless.
export function runSubject(run: AgentRunSummary): string {
  if (run.task_type === "scout_run") {
    return run.scout_name ? `Scout “${run.scout_name}”` : "Deleted scout";
  }
  return taskLabel(run.task_type);
}

/// ACTION_LABELS reads each step the daemon records as a short sentence.
/// A step not listed here is shown by its recorded name, so a step added by
/// a later daemon is still displayed rather than dropped.
const ACTION_LABELS: Record<string, string> = {
  scout_started: "Started",
  source_unreachable: "Could not reach a job source",
  source_failed: "A job source answered with an error",
  sources_searched: "Searched the job sources",
  jobs_collected: "Collected jobs to consider",
  job_rejected: "Passed over a job",
  job_scoring_failed: "Could not score a job",
  application_created: "Created an application",
  application_failed: "Could not create an application",
  ceiling_reached: "Reached the per-run limit",
  scout_finished: "Finished",
};

export function actionLabel(action: string): string {
  return ACTION_LABELS[action] ?? action.replace(/_/g, " ");
}

/// formatDuration renders milliseconds at the precision a person reads:
/// "340 ms", "4.2 s", "3 min 5 s".
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`;
  const seconds = ms / 1000;
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)} s`;
  const minutes = Math.floor(seconds / 60);
  const rest = Math.round(seconds % 60);
  return rest ? `${minutes} min ${rest} s` : `${minutes} min`;
}

/// runDuration is how long a run took, or undefined while it is running.
export function runDuration(run: { started_at: string; completed_at?: string }): string | undefined {
  if (!run.completed_at) return undefined;
  return formatDuration(new Date(run.completed_at).getTime() - new Date(run.started_at).getTime());
}
