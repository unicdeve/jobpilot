import { useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Ellipsis, Plus, Radar, TriangleAlert } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ConfirmDialog } from "../components/confirm";
import { EmptyState } from "../components/empty-state";
import { FormField } from "../components/form-field";
import { Page, Panel, Section } from "../components/page";
import { StatusBadge } from "../components/status-badge";
import { Orb } from "../components/thinking";
import { Button } from "../components/ui/button";
import { Checkbox } from "../components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../components/ui/dropdown-menu";
import { Input } from "../components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../components/ui/select";
import { runDuration } from "../lib/activity";
import type { ScoutInput } from "../lib/api";
import { dateTime, relativeTime } from "../lib/format";
import {
  keys,
  useCreateScout,
  useDeleteScout,
  useRunScout,
  useSavedSearches,
  useScoutRuns,
  useScouts,
  useUpdateScout,
} from "../lib/queries";
import { alarmProblem, isMac, isScheduled, nextRunLabel, scheduleLabel } from "../lib/scouts";
import { useToast } from "../lib/toast";
import type { SavedSearch, ScheduleKind, Scout, ScoutRunReport, ScoutSchedule, ScoutSource } from "../lib/types";

const SOURCE_LABELS: Record<ScoutSource, string> = {
  local: "Jobs already saved here",
  sources: "Fetch from the job boards first",
};

// The select and the field beside it read as one phrase: "Every 6 hours",
// "Every day at 8:00 AM".
const SCHEDULE_LABELS: Record<ScheduleKind, string> = {
  off: "Only when I run it",
  every: "Every",
  daily: "Every day at",
};

const emptyScout = (savedSearchId: string): ScoutInput => ({
  name: "",
  saved_search_id: savedSearchId,
  min_match_score: 70,
  max_applications_per_run: 10,
  source: "local",
  is_enabled: true,
  schedule: { kind: "off" },
});

const toInput = (scout: Scout): ScoutInput => ({
  name: scout.name,
  saved_search_id: scout.saved_search_id,
  min_match_score: scout.min_match_score,
  max_applications_per_run: scout.max_applications_per_run,
  source: scout.source,
  is_enabled: scout.is_enabled,
  schedule: scout.schedule ?? { kind: "off" },
});

export function Scouts() {
  const { notify } = useToast();
  const queryClient = useQueryClient();
  const [report, setReport] = useState<ScoutRunReport | null>(null);
  const runScout = useRunScout({
    onSuccess: (result) => {
      setReport(result);
      notify(
        result.created > 0
          ? `${result.created} application${result.created === 1 ? "" : "s"} ready for review.`
          : "Nothing cleared this scout's rules.",
        result.created > 0 ? "success" : undefined,
      );
    },
    onError: (err) => notify(String(err)),
  });
  const { data: scouts } = useScouts(runScout.isPending);
  const { data: searches } = useSavedSearches();
  const { data: runs } = useScoutRuns(10);

  // A run the schedule started ends without this screen asking for it, so
  // its end is noticed here and what it made is read again.
  const busy = scouts?.some((scout) => scout.activity) ?? false;
  const wasBusy = useRef(busy);
  useEffect(() => {
    if (wasBusy.current && !busy) {
      void queryClient.invalidateQueries({ queryKey: keys.scoutRuns });
      void queryClient.invalidateQueries({ queryKey: keys.applications.all });
      void queryClient.invalidateQueries({ queryKey: keys.activity.all });
      void queryClient.invalidateQueries({ queryKey: keys.notifications });
    }
    wasBusy.current = busy;
  }, [busy, queryClient]);

  // editing is the scout being edited, or "new"; null when the editor is shut.
  const [editing, setEditing] = useState<Scout | "new" | null>(null);
  const [deleting, setDeleting] = useState<Scout | null>(null);

  const deleteScout = useDeleteScout({
    onSuccess: () => setDeleting(null),
    onError: (err) => notify(String(err)),
  });

  const searchName = (id: string) => searches?.find((s) => s.id === id)?.name ?? "a deleted search";
  const scoutName = (id: string | undefined) => scouts?.find((s) => s.id === id)?.name ?? "A deleted scout";

  // A scout runs a saved search, so without one there is nothing to run.
  if (searches && searches.length === 0) {
    return (
      <EmptyState
        title="No saved searches yet"
        description="A scout runs a saved search on your behalf, so it needs one to work from."
        actionLabel="Create a search"
        actionTo="/settings?section=searches"
      />
    );
  }

  return (
    <Page
      title="Scouts"
      width="wide"
      actions={
        <Button size="sm" onClick={() => setEditing("new")} disabled={!searches}>
          <Plus data-icon="inline-start" />
          New scout
        </Button>
      }
    >
      <p className="max-w-2xl text-sm text-muted-foreground">
        A scout runs one of your saved searches, scores what it finds against your profile, and creates applications for
        you to review, when you ask or on a schedule. It never contacts an employer and never submits.
      </p>

      <Section title="Your scouts" className="mt-6">
        {!scouts || scouts.length === 0 ? (
          <Panel className="flex flex-col items-center gap-2 px-6 py-10 text-center">
            <Radar className="size-8 stroke-[1.25] text-muted-foreground/60" />
            <p className="text-sm font-medium text-subtle-foreground">No scouts yet</p>
            <p className="max-w-sm text-sm text-muted-foreground">
              Create one to have JobPilot shortlist jobs from a saved search for you.
            </p>
          </Panel>
        ) : (
          <Panel className="divide-y divide-border">
            {scouts.map((scout) => {
              const problem = isScheduled(scout) ? alarmProblem(scout.alarm) : undefined;
              const asked = runScout.isPending && runScout.variables === scout.id;
              return (
                <div key={scout.id} className="flex items-center gap-4 px-4 py-3">
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
                      {searchName(scout.saved_search_id)} · score ≥ {scout.min_match_score} · up to{" "}
                      {scout.max_applications_per_run} per run ·{" "}
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
                    onClick={() => runScout.mutate(scout.id)}
                    disabled={runScout.isPending || !scout.is_enabled || scout.activity !== undefined}
                  >
                    {scout.activity === "waiting"
                      ? "Waiting…"
                      : asked || scout.activity === "running"
                        ? "Running…"
                        : "Run now"}
                  </Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={<Button size="icon-sm" variant="ghost" aria-label={`More for ${scout.name}`} />}
                    >
                      <Ellipsis />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => setEditing(scout)}>Edit…</DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onClick={() => setDeleting(scout)}>
                        Delete…
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              );
            })}
          </Panel>
        )}
      </Section>

      {report && <RunReport report={report} onDismiss={() => setReport(null)} />}

      {runs && runs.length > 0 && (
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
      )}

      {editing && searches && (
        <ScoutEditor scout={editing === "new" ? null : editing} searches={searches} onClose={() => setEditing(null)} />
      )}

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={`Delete “${deleting?.name}”?`}
        description="The scout stops, along with its schedule, and its past runs stay in Activity. Applications it created are kept."
        confirmLabel="Delete scout"
        onConfirm={() => deleting && deleteScout.mutate(deleting.id)}
        pending={deleteScout.isPending}
      />
    </Page>
  );
}

function ScoutEditor({
  scout,
  searches,
  onClose,
}: {
  scout: Scout | null;
  searches: SavedSearch[];
  onClose: () => void;
}) {
  const { notify } = useToast();
  const [draft, setDraft] = useState<ScoutInput>(scout ? toInput(scout) : emptyScout(searches[0].id));
  const searchLabels = Object.fromEntries(searches.map((s) => [s.id, s.name]));

  const createScout = useCreateScout({
    onSuccess: () => {
      notify("Scout created.", "success");
      onClose();
    },
    onError: (err) => notify(String(err)),
  });
  const updateScout = useUpdateScout({
    onSuccess: () => {
      notify("Scout saved.", "success");
      onClose();
    },
    onError: (err) => notify(String(err)),
  });

  const save = (event: React.FormEvent) => {
    event.preventDefault();
    if (scout) updateScout.mutate({ id: scout.id, scout: draft });
    else createScout.mutate(draft);
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-lg">
        <form onSubmit={save}>
          <DialogTitle>{scout ? "Edit scout" : "New scout"}</DialogTitle>
          <DialogDescription>A scout creates applications for you to review. It never submits.</DialogDescription>

          <div className="mt-5 flex flex-col gap-4">
            <FormField label="Name" htmlFor="scout-name">
              <Input
                id="scout-name"
                autoFocus
                value={draft.name}
                placeholder="Daily Go roles"
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
            </FormField>

            <FormField
              label="Saved search"
              htmlFor="scout-search"
              hint="The search decides which jobs count, including salary floor, excluded companies and location."
            >
              <Select
                items={searchLabels}
                value={draft.saved_search_id}
                onValueChange={(value) => value && setDraft({ ...draft, saved_search_id: value })}
              >
                <SelectTrigger id="scout-search" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {searches.map((search) => (
                    <SelectItem key={search.id} value={search.id}>
                      {search.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>

            <div className="grid grid-cols-2 gap-3">
              <FormField label="Minimum match score" htmlFor="scout-score">
                <Input
                  id="scout-score"
                  type="number"
                  min={0}
                  max={100}
                  value={draft.min_match_score}
                  onChange={(e) => setDraft({ ...draft, min_match_score: Number(e.target.value) })}
                />
              </FormField>
              <FormField label="Most applications per run" htmlFor="scout-ceiling">
                <Input
                  id="scout-ceiling"
                  type="number"
                  min={1}
                  max={50}
                  value={draft.max_applications_per_run}
                  onChange={(e) => setDraft({ ...draft, max_applications_per_run: Number(e.target.value) })}
                />
              </FormField>
            </div>

            <FormField label="Where to look" htmlFor="scout-source">
              <Select
                items={SOURCE_LABELS}
                value={draft.source}
                onValueChange={(value) => value && setDraft({ ...draft, source: value as ScoutSource })}
              >
                <SelectTrigger id="scout-source" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(SOURCE_LABELS).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>

            <ScheduleField schedule={draft.schedule} onChange={(schedule) => setDraft({ ...draft, schedule })} />

            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={draft.is_enabled}
                onCheckedChange={(checked) => setDraft({ ...draft, is_enabled: checked === true })}
              />
              Enabled
            </label>
          </div>

          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>Cancel</DialogClose>
            <Button
              type="submit"
              disabled={
                !draft.name.trim() || !scheduleValid(draft.schedule) || createScout.isPending || updateScout.isPending
              }
            >
              {scout ? "Save scout" : "Create scout"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/// scheduleValid reports a schedule the daemon will accept: every 1 to 24
/// hours, or a time of day.
function scheduleValid(schedule: ScoutSchedule): boolean {
  switch (schedule.kind) {
    case "every": {
      const hours = schedule.every_hours;
      return hours !== undefined && Number.isInteger(hours) && hours >= 1 && hours <= 24;
    }
    case "daily": {
      const minute = schedule.daily_at;
      return minute !== undefined && Number.isInteger(minute) && minute >= 0 && minute < 24 * 60;
    }
    default:
      return true;
  }
}

const pad = (n: number) => String(n).padStart(2, "0");

/// ScheduleField says when a scout runs on its own: never, every few
/// hours, or once a day at a time on this computer's clock.
function ScheduleField({ schedule, onChange }: { schedule: ScoutSchedule; onChange: (s: ScoutSchedule) => void }) {
  const kind = schedule.kind ?? "off";
  const choose = (next: ScheduleKind) => {
    if (next === "every") onChange({ kind: "every", every_hours: schedule.every_hours ?? 6 });
    else if (next === "daily") onChange({ kind: "daily", daily_at: schedule.daily_at ?? 8 * 60 });
    else onChange({ kind: "off" });
  };
  const dailyAt = schedule.daily_at ?? 8 * 60;

  return (
    <FormField
      label="Runs on its own"
      htmlFor="scout-schedule"
      hint={
        kind === "off"
          ? undefined
          : `Your computer starts it, even with JobPilot closed, while you’re logged in and it’s awake. A run missed while it was off or asleep happens once when it’s back.${
              isMac() ? " macOS will show that JobPilot added a background item." : ""
            }`
      }
    >
      <div className="flex items-center gap-3">
        <Select items={SCHEDULE_LABELS} value={kind} onValueChange={(value) => value && choose(value as ScheduleKind)}>
          <SelectTrigger id="scout-schedule" className={kind === "off" ? "min-w-0 flex-1" : "w-44"}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {Object.entries(SCHEDULE_LABELS).map(([value, label]) => (
              <SelectItem key={value} value={value}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {kind === "every" && (
          <div className="flex shrink-0 items-center gap-2">
            <Input
              aria-label="Hours between runs"
              type="number"
              min={1}
              max={24}
              className="w-16"
              value={Number.isNaN(schedule.every_hours) ? "" : schedule.every_hours}
              onChange={(e) => onChange({ kind: "every", every_hours: e.target.valueAsNumber })}
            />
            <span className="text-sm text-muted-foreground">hours</span>
          </div>
        )}
        {kind === "daily" && (
          <Input
            aria-label="Time of day"
            type="time"
            className="w-34"
            value={Number.isNaN(dailyAt) ? "" : `${pad(Math.floor(dailyAt / 60))}:${pad(dailyAt % 60)}`}
            onChange={(e) => {
              const [hours, minutes] = e.target.value.split(":").map(Number);
              onChange({ kind: "daily", daily_at: e.target.value ? hours * 60 + minutes : Number.NaN });
            }}
          />
        )}
      </div>
    </FormField>
  );
}

/// RunReport shows what a run just did, including why each job was
/// rejected — the run's reasoning is the point, not just its output.
function RunReport({ report, onDismiss }: { report: ScoutRunReport; onDismiss: () => void }) {
  const verdicts = report.verdicts ?? [];
  return (
    <Section
      title={`Considered ${report.considered} job${report.considered === 1 ? "" : "s"}, created ${report.created} application${report.created === 1 ? "" : "s"}`}
      description={
        report.error ? (
          <span className="text-destructive">{report.error}</span>
        ) : report.ceiling_hit ? (
          "Stopped at this scout’s per-run limit. Run it again for more."
        ) : report.created > 0 ? (
          <>
            <Link className="text-foreground underline" to="/applications">
              Review them
            </Link>{" "}
            — nothing has been sent.
          </>
        ) : undefined
      }
      actions={
        <Button size="sm" variant="ghost" onClick={onDismiss}>
          Dismiss
        </Button>
      }
    >
      {verdicts.length > 0 && (
        <Panel className="divide-y divide-border">
          {verdicts.map((verdict) => (
            <div key={verdict.job_id} className="flex items-start gap-3 px-4 py-2">
              <StatusBadge value={verdict.eligible ? "allowed" : "denied"} className="mt-0.5 shrink-0" />
              <p className="min-w-0 text-[13px]">
                <Link to={`/jobs/${verdict.job_id}`} className="font-medium hover:underline">
                  {verdict.job_title}
                </Link>
                <span className="text-muted-foreground"> — {verdict.reason}</span>
              </p>
            </div>
          ))}
        </Panel>
      )}
    </Section>
  );
}
