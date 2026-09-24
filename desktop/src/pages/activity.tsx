import { useState } from "react";
import { Link } from "react-router-dom";
import { EmptyState, fillsContentArea } from "../components/empty-state";
import { FormField } from "../components/form-field";
import { LoadMore } from "../components/load-more";
import { StatusBadge } from "../components/status-badge";
import { Card, CardContent } from "../components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import { runDuration, runSubject, TASK_LABELS, taskLabel } from "../lib/activity";
import { useInfiniteActivity } from "../lib/queries";
import type { AgentRunFilter, AgentRunStatus, AgentTaskType } from "../lib/types";

const PAGE_SIZE = 50;

/// ALL stands for "no filter" in the selects, which cannot hold an empty
/// string as a distinct choice.
const ALL = "all";

const KIND_ITEMS: Record<string, string> = { [ALL]: "All kinds", ...TASK_LABELS };

const STATUS_ITEMS: Record<string, string> = {
  [ALL]: "Any status",
  running: "Running",
  completed: "Completed",
  blocked: "Blocked",
  failed: "Failed",
};

/// Activity lists every agent run, of every kind, newest first
/// (docs/02_features/12_agent_activity.md). Each run's own screen shows the
/// steps it recorded.
export function Activity() {
  const [filter, setFilter] = useState<AgentRunFilter>({ task_type: "", status: "" });
  const { data, error, fetchNextPage, hasNextPage, isFetchingNextPage, isPending } =
    useInfiniteActivity(filter, PAGE_SIZE);

  const runs = data?.pages.flatMap((page) => page.runs) ?? [];
  const total = data?.pages[0]?.total ?? 0;
  const filtered = filter.task_type !== "" || filter.status !== "";

  // With no runs at all there is nothing to filter, so the whole screen
  // explains what will appear here instead.
  if (!isPending && !error && total === 0 && !filtered) {
    return (
      <div className={`flex flex-col ${fillsContentArea}`}>
        <h1 className="text-xl font-semibold">Activity</h1>
        <EmptyState
          className="min-h-0 flex-1"
          title="Nothing has run yet"
          description="Each time JobPilot prepares an application in the browser or runs a scout, what it did is recorded here, step by step."
          actionLabel="Go to Scouts"
          actionTo="/scouts"
        />
      </div>
    );
  }

  return (
    <div>
      <h1 className="text-xl font-semibold">Activity</h1>
      <p className="mt-1 mb-4 text-sm text-slate-500">
        What JobPilot did on your behalf, and why it stopped where it did.
      </p>

      <Card className="mb-4">
        <CardContent className="flex flex-wrap items-end gap-3">
          <div className="w-52">
            <FormField label="Kind" htmlFor="activity-kind">
              <Select
                items={KIND_ITEMS}
                value={filter.task_type || ALL}
                onValueChange={(value) =>
                  setFilter({ ...filter, task_type: value && value !== ALL ? (value as AgentTaskType) : "" })
                }
              >
                <SelectTrigger id="activity-kind" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(KIND_ITEMS).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
          </div>
          <div className="w-52">
            <FormField label="Status" htmlFor="activity-status">
              <Select
                items={STATUS_ITEMS}
                value={filter.status || ALL}
                onValueChange={(value) =>
                  setFilter({ ...filter, status: value && value !== ALL ? (value as AgentRunStatus) : "" })
                }
              >
                <SelectTrigger id="activity-status" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(STATUS_ITEMS).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
          </div>
        </CardContent>
      </Card>

      {error ? (
        <p className="text-sm text-red-600">{String(error.message)}</p>
      ) : isPending ? (
        <p className="text-sm text-slate-500">Loading…</p>
      ) : runs.length === 0 ? (
        <p className="text-sm text-slate-500">No runs match these filters.</p>
      ) : (
        <div className="space-y-2">
          <p className="text-xs text-slate-500">
            {runs.length === total
              ? `${total} ${total === 1 ? "run" : "runs"}`
              : `Showing ${runs.length} of ${total} runs`}
          </p>
          {runs.map((run) => {
            const duration = runDuration(run);
            return (
              <Link key={run.id} to={`/activity/${run.id}`} className="block">
                <Card className="hover:ring-foreground/20">
                  <CardContent>
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate font-medium">{runSubject(run)}</p>
                        <p className="mt-1 text-xs text-slate-500">
                          {taskLabel(run.task_type)} · {new Date(run.started_at).toLocaleString()}
                          {duration && <> · took {duration}</>} · {run.event_count}{" "}
                          {run.event_count === 1 ? "step" : "steps"}
                        </p>
                      </div>
                      <StatusBadge value={run.status} />
                    </div>
                    {run.error && <p className="mt-2 text-sm text-red-700">{run.error}</p>}
                  </CardContent>
                </Card>
              </Link>
            );
          })}
          <LoadMore
            hasNextPage={hasNextPage}
            isFetchingNextPage={isFetchingNextPage}
            fetchNextPage={fetchNextPage}
          />
        </div>
      )}
    </div>
  );
}
