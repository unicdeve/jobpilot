// Wording for the agent trail, shared by the Activity screens and the
// per-application and per-scout views that link to them.

import type { AgentRunSummary, AgentTaskType } from "./types";

export const TASK_LABELS: Record<AgentTaskType, string> = {
  prepare_application: "Prepared in browser",
  scout_run: "Scout run",
};

export function taskLabel(taskType: string): string {
  return TASK_LABELS[taskType as AgentTaskType] ?? taskType;
}

/// runSubject names what a run served: its scout, or its application's job.
/// Either can be deleted after the run, which keeps its record regardless.
export function runSubject(run: AgentRunSummary): string {
  if (run.task_type === "scout_run") {
    return run.scout_name ? `Scout “${run.scout_name}”` : "Deleted scout";
  }
  if (run.task_type === "prepare_application") {
    if (!run.job_title) return "Deleted application";
    return run.company ? `${run.job_title} at ${run.company}` : run.job_title;
  }
  return taskLabel(run.task_type);
}

/// ACTION_LABELS reads each step the daemon records as a short sentence.
/// A step not listed here is shown by its recorded name, so a step added by
/// a later daemon is still displayed rather than dropped.
const ACTION_LABELS: Record<string, string> = {
  navigate: "Opened the job posting",
  detect_blocker: "Stopped at a blocker",
  extract_form: "Read the page",
  follow_apply: "Followed an apply link",
  choose_apply: "Asked the model for the way in",
  extract_question: "Found a question",
  resolve_question: "Worked out an answer",
  skip_field: "Left a field for you",
  fill_field: "Filled a field",
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

/// ANSWER_SOURCES reads a resolve_question step's output, which is where
/// the answer came from rather than what it was.
const ANSWER_SOURCES: Record<string, string> = {
  profile_fact: "From your profile",
  derived: "Derived from your profile",
  generated: "Written by the model",
  needs_user_input: "Left for you to answer",
};

/// stepOutput is a step's recorded output as a person reads it.
export function stepOutput(action: string, output: string | undefined): string | undefined {
  if (!output) return undefined;
  if (action === "resolve_question") return ANSWER_SOURCES[output] ?? output;
  if (action === "extract_question") return `A ${output} field`;
  return output;
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
