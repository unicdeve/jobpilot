import { ChevronRight } from "lucide-react";
import { Link } from "react-router-dom";
import { Panel, Section } from "../../components/page";
import { StatusBadge } from "../../components/status-badge";
import { runDuration } from "../../lib/activity";
import { dateTime, relativeTime } from "../../lib/format";
import type { AgentRun } from "../../lib/types";

/// RecentRuns lists the latest scout runs, each linking to what it did.
export function RecentRuns({ runs, scoutName }: { runs: AgentRun[]; scoutName: (id: string | undefined) => string }) {
  return (
    <Section
      title="Recent runs"
      actions={
        <Link to="/activity?kind=scout_run" className="text-xs text-muted-foreground hover:text-foreground">
          All scout runs
        </Link>
      }
    >
      <Panel className="divide-y divide-border">
        {runs.map((run) => {
          const duration = runDuration(run);
          return (
            <Link
              key={run.id}
              to={`/activity/${run.id}`}
              className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/50"
            >
              <div className="min-w-0 flex-1">
                <p className="text-[13px]">
                  <span className="font-medium">{scoutName(run.scout_id)}</span>
                  <span className="text-muted-foreground"> · </span>
                  <span title={dateTime(run.started_at)}>{relativeTime(run.started_at)}</span>
                  {run.trigger === "schedule" && <span className="text-muted-foreground"> · on schedule</span>}
                  {duration && <span className="text-muted-foreground"> · took {duration}</span>}
                </p>
                {run.error && <p className="mt-0.5 truncate text-xs text-destructive">{run.error}</p>}
              </div>
              <StatusBadge value={run.status} />
              <ChevronRight className="size-4 text-muted-foreground" />
            </Link>
          );
        })}
      </Panel>
    </Section>
  );
}
