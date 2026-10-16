import { Ellipsis, TriangleAlert } from "lucide-react";
import { Orb } from "../../components/thinking";
import { Button } from "../../components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import { dateTime, relativeTime } from "../../lib/format";
import { alarmProblem, isScheduled, nextRunLabel, scheduleLabel } from "../../lib/scouts";
import type { Scout } from "../../lib/types";

interface ScoutRowProps {
  scout: Scout;
  /// searchName is the name of the saved search the scout runs.
  searchName: string;
  /// asked is whether this screen asked this scout to run, before the
  /// daemon reports it running.
  asked: boolean;
  /// runDisabled is set while any run asked for here is under way.
  runDisabled: boolean;
  onRun: () => void;
  onEdit: () => void;
  onDelete: () => void;
}

/// ScoutRow is one scout: what it runs, when, how it last went, and the
/// actions on it.
export function ScoutRow({ scout, searchName, asked, runDisabled, onRun, onEdit, onDelete }: ScoutRowProps) {
  const problem = isScheduled(scout) ? alarmProblem(scout.alarm) : undefined;
  return (
    <div className="flex items-center gap-4 px-4 py-3">
      <div className="min-w-0 flex-1">
        <p className="flex items-center gap-2 text-[13px] font-medium">
          <span className="truncate">{scout.name}</span>
          {!scout.is_enabled && (
            <span className="rounded bg-muted px-1.5 py-px text-[11px] leading-4 font-normal text-muted-foreground">
              Paused
            </span>
          )}
          {/* Running is work done for the user, so it carries the
                      orb; waiting is not work yet, and is only a state. */}
          {scout.activity === "running" && (
            <span className="-my-0.5 inline-flex items-center gap-1 text-xs font-normal text-muted-foreground">
              <Orb state="working" />
              Running
            </span>
          )}
          {scout.activity === "waiting" && (
            <span className="rounded bg-muted px-1.5 py-px text-[11px] leading-4 font-normal text-muted-foreground">
              Waiting for another scout
            </span>
          )}
        </p>
        <p className="mt-0.5 text-xs text-subtle-foreground">
          {searchName} · score ≥ {scout.min_match_score} · up to {scout.max_applications_per_run} per run ·{" "}
          {scout.source === "local" ? "saved jobs" : "fetches from the job boards"}
        </p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          Runs {scheduleLabel(scout.schedule)}
          {isScheduled(scout) && scout.next_run_at && !problem && (
            <span title={dateTime(scout.next_run_at)}>, next {nextRunLabel(scout.next_run_at)}</span>
          )}
          {" · "}
          {scout.last_run_at ? (
            <span title={dateTime(scout.last_run_at)}>last ran {relativeTime(scout.last_run_at)}</span>
          ) : (
            "hasn’t run yet"
          )}
        </p>
        {problem && (
          <p className="mt-1 flex items-start gap-1.5 text-xs text-warning">
            <TriangleAlert className="mt-px size-3.5 shrink-0" />
            {problem}
          </p>
        )}
      </div>
      <Button
        size="sm"
        variant="outline"
        onClick={onRun}
        disabled={runDisabled || !scout.is_enabled || scout.activity !== undefined}
      >
        {scout.activity === "waiting" ? "Waiting…" : asked || scout.activity === "running" ? "Running…" : "Run now"}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button size="icon-sm" variant="ghost" aria-label={`More for ${scout.name}`} />}>
          <Ellipsis />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={onEdit}>Edit…</DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            Delete…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
