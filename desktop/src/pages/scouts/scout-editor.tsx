import { useState } from "react";
import { FormField } from "../../components/form-field";
import { Button } from "../../components/ui/button";
import { Checkbox } from "../../components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../../components/ui/dialog";
import { Input } from "../../components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../components/ui/select";
import type { ScoutInput } from "../../lib/api";
import { useCreateScout, useUpdateScout } from "../../lib/queries/scout.queries";
import { isMac } from "../../lib/scouts";
import { useToast } from "../../lib/toast";
import type { SavedSearch, ScheduleKind, Scout, ScoutSchedule, ScoutSource } from "../../lib/types";

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

export function ScoutEditor({
  scout,
  searches,
  onClose,
}: {
  scout: Scout | null;
  searches: SavedSearch[];
  onClose: () => void;
}) {
  const { notify, notifyError } = useToast();
  const [draft, setDraft] = useState<ScoutInput>(scout ? toInput(scout) : emptyScout(searches[0].id));
  const searchLabels = Object.fromEntries(searches.map((s) => [s.id, s.name]));

  const createScout = useCreateScout({
    onSuccess: () => {
      notify("Scout created.", "success");
      onClose();
    },
    onError: notifyError,
  });
  const updateScout = useUpdateScout({
    onSuccess: () => {
      notify("Scout saved.", "success");
      onClose();
    },
    onError: notifyError,
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
