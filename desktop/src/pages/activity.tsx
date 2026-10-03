import { History } from "lucide-react";
import { Outlet, useMatch, useSearchParams } from "react-router-dom";
import { ListItem, ListNote, ListPane, NoSelection } from "../components/list-pane";
import { LoadMore } from "../components/load-more";
import { SplitView } from "../components/split-view";
import { FILTER_ALL as ALL, FilterSelect } from "../components/filter-select";
import { StatusBadge } from "../components/status-badge";
import { runDuration, runKind, runSubject, TASK_LABELS } from "../lib/activity";
import { dateTime, relativeTime } from "../lib/format";
import { useInfiniteActivity } from "../lib/queries";
import type { AgentRunFilter, AgentRunStatus, AgentRunSummary, AgentTaskType } from "../lib/types";

const PAGE_SIZE = 50;

const KIND_ITEMS: Record<string, string> = { [ALL]: "All kinds", ...TASK_LABELS };

const STATUS_ITEMS: Record<string, string> = {
  [ALL]: "Any status",
  running: "Running",
  completed: "Completed",
  blocked: "Blocked",
  failed: "Failed",
};

/// Activity lists every agent run, of every kind, newest first, beside the
/// steps of the one selected (docs/02_features/12_agent_activity.md). The
/// filters live in the URL, so a link can open a filtered list.
export function Activity() {
  const hasSelection = useMatch("/activity/:id") !== null;
  const [params, setParams] = useSearchParams();
  const kind = params.get("kind");
  const status = params.get("status");
  const filter: AgentRunFilter = {
    task_type: kind && kind in TASK_LABELS ? (kind as AgentTaskType) : "",
    status: status && status in STATUS_ITEMS && status !== ALL ? (status as AgentRunStatus) : "",
  };

  const setParam = (key: string, value: string) =>
    setParams(
      (current) => {
        const next = new URLSearchParams(current);
        if (value) next.set(key, value);
        else next.delete(key);
        return next;
      },
      { replace: true },
    );

  return (
    <SplitView
      hasSelection={hasSelection}
      list={
        <ListPane
          title="Activity"
          controls={
            <div className="flex gap-2">
              <FilterSelect
                label="Kind"
                items={KIND_ITEMS}
                value={filter.task_type}
                onChange={(value) => setParam("kind", value)}
                className="flex-1"
              />
              <FilterSelect
                label="Status"
                items={STATUS_ITEMS}
                value={filter.status}
                onChange={(value) => setParam("status", value)}
                className="flex-1"
              />
            </div>
          }
        >
          <RunList filter={filter} />
        </ListPane>
      }
      detail={<Outlet />}
    />
  );
}

export function NoRunSelected() {
  return (
    <NoSelection
      icon={<History />}
      title="No run selected"
      description="Choose a run to see each step it took, and why it stopped where it did."
    />
  );
}

function RunList({ filter }: { filter: AgentRunFilter }) {
  const { data, error, fetchNextPage, hasNextPage, isFetchingNextPage, isPending } = useInfiniteActivity(
    filter,
    PAGE_SIZE,
  );

  if (error) return <ListNote tone="error">{error.message}</ListNote>;
  if (isPending) return <ListNote>Loading…</ListNote>;

  const runs = data.pages.flatMap((page) => page.runs);
  const total = data.pages[0]?.total ?? 0;
  const filtered = filter.task_type !== "" || filter.status !== "";

  if (runs.length === 0) {
    return (
      <ListNote>
        {filtered
          ? "No runs match these filters."
          : "Nothing has run yet. Each time JobPilot prepares an application in the browser or runs a scout, what it did is recorded here."}
      </ListNote>
    );
  }

  return (
    <>
      <ListNote>
        {total} {total === 1 ? "run" : "runs"}
      </ListNote>
      <div className="border-t border-border">
        {runs.map((run) => (
          <RunRow key={run.id} run={run} />
        ))}
      </div>
      {hasNextPage && (
        <div className="px-4 pb-4">
          <LoadMore hasNextPage={hasNextPage} isFetchingNextPage={isFetchingNextPage} fetchNextPage={fetchNextPage} />
        </div>
      )}
    </>
  );
}

function RunRow({ run }: { run: AgentRunSummary }) {
  const duration = runDuration(run);
  return (
    <ListItem to={`/activity/${run.id}`}>
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="line-clamp-2 text-[13px] leading-snug font-medium">{runSubject(run)}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {runKind(run)} · <span title={dateTime(run.started_at)}>{relativeTime(run.started_at)}</span>
            {duration && <> · {duration}</>}
          </p>
          {run.error && <p className="mt-1 line-clamp-2 text-xs text-destructive">{run.error}</p>}
        </div>
        <StatusBadge value={run.status} className="mt-0.5 shrink-0" />
      </div>
    </ListItem>
  );
}
