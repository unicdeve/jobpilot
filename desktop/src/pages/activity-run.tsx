import { Link, useParams } from "react-router-dom";
import { StatusBadge } from "../components/status-badge";
import { Card, CardContent, CardTitle } from "../components/ui/card";
import {
  actionLabel,
  formatDuration,
  runDuration,
  runSubject,
  stepOutput,
  taskLabel,
} from "../lib/activity";
import { useActivityRun } from "../lib/queries";
import type { AgentEvent, AgentRunSummary } from "../lib/types";

/// ActivityRun shows one agent run and every step it recorded, in order.
///
/// The steps are what the run did and what each deterministic rule decided,
/// never the model's reasoning, and never the answers typed into a form:
/// those are on the application's questions
/// (docs/02_features/12_agent_activity.md).
export function ActivityRun() {
  const { id } = useParams<{ id: string }>();
  const { data, error } = useActivityRun(id);

  if (error) return <p className="text-sm text-red-600">{String(error.message)}</p>;
  if (!data) return <p className="text-sm text-slate-500">Loading…</p>;

  const { run, events } = data;

  return (
    <div className="max-w-3xl space-y-4">
      <RunHeader run={run} />

      {run.error && (
        <Card>
          <CardContent>
            <CardTitle>Why it stopped</CardTitle>
            <p className="mt-1 text-sm text-red-700">{run.error}</p>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent>
          <CardTitle>Steps</CardTitle>
          {events.length === 0 ? (
            <p className="mt-2 text-sm text-slate-500">No steps recorded.</p>
          ) : (
            <ol className="mt-2 divide-y divide-slate-100">
              {events.map((event) => (
                <Step key={event.id} event={event} />
              ))}
            </ol>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function RunHeader({ run }: { run: AgentRunSummary }) {
  const duration = runDuration(run);
  return (
    <div>
      <Link to="/activity" className="text-sm text-slate-500 hover:underline">
        ← Activity
      </Link>
      <div className="mt-1 flex items-start gap-3">
        <h1 className="text-xl font-semibold">{runSubject(run)}</h1>
        <StatusBadge value={run.status} className="mt-1 shrink-0" />
      </div>
      <p className="text-sm text-slate-600">
        {taskLabel(run.task_type)} · {new Date(run.started_at).toLocaleString()}
        {duration ? <> · took {duration}</> : run.status === "running" && <> · still running</>}
        <SubjectLink run={run} />
      </p>
      <p className="mt-1 font-mono text-xs text-slate-400">{run.id}</p>
    </div>
  );
}

/// SubjectLink leads to what the run served, when it still exists.
function SubjectLink({ run }: { run: AgentRunSummary }) {
  if (run.task_type === "prepare_application" && run.application_id && run.job_title) {
    return (
      <>
        {" · "}
        <Link to={`/applications/${run.application_id}`} className="hover:underline">
          View application
        </Link>
      </>
    );
  }
  if (run.task_type === "scout_run" && run.scout_id && run.scout_name) {
    return (
      <>
        {" · "}
        <Link to="/scouts" className="hover:underline">
          View scouts
        </Link>
      </>
    );
  }
  return null;
}

function Step({ event }: { event: AgentEvent }) {
  const failed = event.status !== "" && event.status !== "ok";
  const output = stepOutput(event.action, event.output_summary);
  return (
    <li className="flex items-start justify-between gap-4 py-2.5 first:pt-1 last:pb-0">
      <div className="min-w-0">
        <p className="text-sm font-medium">
          {actionLabel(event.action)}
          {event.input_summary && (
            <span className="font-normal text-slate-600">: {event.input_summary}</span>
          )}
        </p>
        {output && (
          <p className={`mt-0.5 text-sm break-words ${failed ? "text-red-700" : "text-slate-500"}`}>
            {output}
          </p>
        )}
        <p className="mt-0.5 text-xs text-slate-400">
          <span className="font-mono">{event.action}</span>
          {event.tool && <> · {event.tool}</>}
          {event.duration_ms > 0 && <> · {formatDuration(event.duration_ms)}</>}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        {event.policy_result && <StatusBadge value={event.policy_result} />}
        {failed && <StatusBadge value={event.status} />}
      </div>
    </li>
  );
}
