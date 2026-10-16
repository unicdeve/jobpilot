import { ArrowLeft, ChevronRight, History } from "lucide-react";
import { Link, useLocation, useParams } from "react-router-dom";
import { RecordGone } from "../components/list-pane";
import { DetailSection, Toolbar } from "../components/page";
import { splitBackClass } from "../components/split-view";
import { StatusBadge } from "../components/status-badge";
import { ErrorLine, Loading } from "../components/status-line";
import { Button } from "../components/ui/button";
import { actionLabel, formatDuration, runDuration, runKind, runSubject, stepOutput } from "../lib/activity";
import { isNotFound } from "../lib/api";
import { dateTime } from "../lib/format";
import { useActivityRun } from "../lib/queries/activity.queries";
import type { AgentEvent, AgentRunSummary } from "../lib/types";

/// ActivityRun shows one agent run and every step it recorded, in order.
///
/// The steps are what the run did and what each deterministic rule decided,
/// never the model's reasoning, and never the answers typed into a form:
/// those are on the application's questions
/// (docs/02_features/12_agent_activity.md).
export function ActivityRun() {
  const { id } = useParams<{ id: string }>();
  const { search } = useLocation();
  const { data, error } = useActivityRun(id);

  const back = (
    <Button
      size="icon-sm"
      variant="ghost"
      className={splitBackClass}
      aria-label="Back to activity"
      render={<Link to={{ pathname: "/activity", search }} />}
    >
      <ArrowLeft />
    </Button>
  );

  if (isNotFound(error)) {
    return (
      <RecordGone
        back={back}
        icon={<History />}
        title="This run no longer exists"
        description="Activity may have been cleared. Choose another run from the list."
      />
    );
  }
  if (error) return <ErrorLine error={error} />;
  if (!data) return <Loading />;

  const { run, events } = data;
  const duration = runDuration(run);

  return (
    <div className="min-h-full">
      <Toolbar className="px-5" leading={back} actions={<SubjectLink run={run} />} />

      <article className="max-w-3xl px-8 pt-6 pb-12">
        <header>
          <div className="flex items-start gap-3">
            <h2 className="min-w-0 flex-1 text-xl leading-tight font-semibold">{runSubject(run)}</h2>
            <StatusBadge value={run.status} className="mt-1 shrink-0" />
          </div>
          <p className="mt-1 text-sm text-subtle-foreground">
            {runKind(run)} · {dateTime(run.started_at)}
            {duration ? <> · took {duration}</> : run.status === "running" && <> · still running</>}
          </p>
          <p className="mt-1 font-mono text-[11px] text-muted-foreground select-text">{run.id}</p>
        </header>

        {run.error && (
          <div className="mt-6 rounded-lg border border-destructive/30 bg-destructive/8 px-4 py-3">
            <p className="text-sm font-medium text-destructive">Why it stopped</p>
            <p className="mt-0.5 text-sm text-subtle-foreground select-text">{run.error}</p>
          </div>
        )}

        <DetailSection title="Steps" description={`${events.length} recorded, in order.`}>
          {events.length === 0 ? (
            <p className="text-sm text-muted-foreground">No steps recorded.</p>
          ) : (
            <ol className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-card">
              {events.map((event) => (
                <Step key={event.id} event={event} />
              ))}
            </ol>
          )}
        </DetailSection>
      </article>
    </div>
  );
}

/// SubjectLink leads to what the run served, when it still exists.
function SubjectLink({ run }: { run: AgentRunSummary }) {
  if (run.task_type === "prepare_application" && run.application_id && run.job_title) {
    return (
      <Button size="sm" variant="ghost" render={<Link to={`/applications/${run.application_id}`} />}>
        View application
        <ChevronRight data-icon="inline-end" />
      </Button>
    );
  }
  if (run.task_type === "scout_run" && run.scout_id && run.scout_name) {
    return (
      <Button size="sm" variant="ghost" render={<Link to="/scouts" />}>
        View scouts
        <ChevronRight data-icon="inline-end" />
      </Button>
    );
  }
  return null;
}

function Step({ event }: { event: AgentEvent }) {
  const failed = event.status !== "" && event.status !== "ok";
  const output = stepOutput(event.action, event.output_summary);
  return (
    <li className="flex items-start justify-between gap-4 px-4 py-2.5">
      <div className="min-w-0">
        <p className="text-[13px] font-medium">
          {actionLabel(event.action)}
          {event.input_summary && (
            <span className="font-normal break-words text-subtle-foreground select-text">: {event.input_summary}</span>
          )}
        </p>
        {output && (
          <p
            className={`mt-0.5 text-[13px] break-words select-text ${failed ? "text-destructive" : "text-muted-foreground"}`}
          >
            {output}
          </p>
        )}
        <p className="mt-0.5 text-xs text-muted-foreground">
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
