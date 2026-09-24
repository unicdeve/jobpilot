import { useState } from "react";
import { Link } from "react-router-dom";
import { EmptyState } from "../components/empty-state";
import { FormField } from "../components/form-field";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Input } from "../components/ui/input";
import {
  useCreateScout,
  useDeleteScout,
  useRunScout,
  useSavedSearches,
  useScoutRunEvents,
  useScoutRuns,
  useScouts,
  useUpdateScout,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type { Scout, ScoutRunReport, ScoutSource } from "../lib/types";
import type { ScoutInput } from "../lib/api";

const selectClass =
  "h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm text-slate-900";

const emptyScout = (savedSearchId: string): ScoutInput => ({
  name: "",
  saved_search_id: savedSearchId,
  min_match_score: 70,
  max_applications_per_run: 10,
  source: "local",
  is_enabled: true,
});

export function Scouts() {
  const { notify } = useToast();
  const { data: scouts } = useScouts();
  const { data: searches } = useSavedSearches();
  const { data: runs } = useScoutRuns(10);

  const [draft, setDraft] = useState<ScoutInput | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [report, setReport] = useState<ScoutRunReport | null>(null);
  const [openRunId, setOpenRunId] = useState<string | null>(null);

  const createScout = useCreateScout({
    onSuccess: () => {
      notify("Scout created.", "success");
      setDraft(null);
    },
    onError: (err) => notify(String(err)),
  });
  const updateScout = useUpdateScout({
    onSuccess: () => {
      notify("Scout saved.", "success");
      setDraft(null);
      setEditingId(null);
    },
    onError: (err) => notify(String(err)),
  });
  const deleteScout = useDeleteScout({ onError: (err) => notify(String(err)) });
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

  const searchName = (id: string) => searches?.find((s) => s.id === id)?.name ?? "a deleted search";

  const startCreating = () => {
    if (!searches?.length) return;
    setEditingId(null);
    setDraft(emptyScout(searches[0].id));
  };

  const startEditing = (scout: Scout) => {
    setEditingId(scout.id);
    setDraft({
      name: scout.name,
      saved_search_id: scout.saved_search_id,
      min_match_score: scout.min_match_score,
      max_applications_per_run: scout.max_applications_per_run,
      source: scout.source,
      is_enabled: scout.is_enabled,
    });
  };

  const save = () => {
    if (!draft) return;
    if (editingId) {
      updateScout.mutate({ id: editingId, scout: draft });
      return;
    }
    createScout.mutate(draft);
  };

  // A scout runs a saved search, so without one there is nothing to run.
  if (!searches || searches.length === 0) {
    return (
      <EmptyState
        title="No saved searches yet"
        description="A scout runs a saved search on your behalf, so it needs one to work from."
        actionLabel="Create a search"
        actionTo="/jobs"
      />
    );
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Scouts</h1>
          <p className="mt-1 text-sm text-slate-500">
            A scout runs a saved search, scores what it finds against your profile, and creates
            applications for you to review. It never contacts an employer and never submits.
          </p>
        </div>
        <Button onClick={startCreating}>New scout</Button>
      </div>

      {draft && (
        <Card className="mb-4">
          <CardContent className="space-y-3">
            <h2 className="font-medium">{editingId ? "Edit scout" : "New scout"}</h2>
            <FormField label="Name" htmlFor="scout-name">
              <Input
                id="scout-name"
                value={draft.name}
                placeholder="Daily Go roles"
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
            </FormField>

            <FormField label="Saved search" htmlFor="scout-search">
              <select
                id="scout-search"
                className={selectClass}
                value={draft.saved_search_id}
                onChange={(e) => setDraft({ ...draft, saved_search_id: e.target.value })}
              >
                {searches.map((search) => (
                  <option key={search.id} value={search.id}>
                    {search.name}
                  </option>
                ))}
              </select>
              <p className="mt-1 text-xs text-slate-500">
                The search decides which jobs count — including salary floor, excluded companies
                and location. Change those on the search itself.
              </p>
            </FormField>

            <div className="grid grid-cols-2 gap-3">
              <FormField label="Minimum match score" htmlFor="scout-score">
                <Input
                  id="scout-score"
                  type="number"
                  min={0}
                  max={100}
                  value={draft.min_match_score}
                  onChange={(e) =>
                    setDraft({ ...draft, min_match_score: Number(e.target.value) })
                  }
                />
              </FormField>
              <FormField label="Most applications per run" htmlFor="scout-ceiling">
                <Input
                  id="scout-ceiling"
                  type="number"
                  min={1}
                  max={50}
                  value={draft.max_applications_per_run}
                  onChange={(e) =>
                    setDraft({ ...draft, max_applications_per_run: Number(e.target.value) })
                  }
                />
              </FormField>
            </div>

            <FormField label="Where to look" htmlFor="scout-source">
              <select
                id="scout-source"
                className={selectClass}
                value={draft.source}
                onChange={(e) => setDraft({ ...draft, source: e.target.value as ScoutSource })}
              >
                <option value="local">Jobs already saved here</option>
                <option value="sources">Fetch from the job boards first</option>
              </select>
            </FormField>

            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={draft.is_enabled}
                onChange={(e) => setDraft({ ...draft, is_enabled: e.target.checked })}
              />
              Enabled
            </label>

            <div className="flex gap-2">
              <Button onClick={save} disabled={createScout.isPending || updateScout.isPending}>
                {editingId ? "Save scout" : "Create scout"}
              </Button>
              <Button
                variant="secondary"
                onClick={() => {
                  setDraft(null);
                  setEditingId(null);
                }}
              >
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {!scouts || scouts.length === 0 ? (
        <p className="text-sm text-slate-500">
          No scouts yet. Create one to have JobPilot shortlist jobs for you.
        </p>
      ) : (
        <div className="space-y-2">
          {scouts.map((scout) => (
            <Card key={scout.id}>
              <CardContent className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <p className="font-medium">
                    {scout.name}
                    {!scout.is_enabled && (
                      <span className="ml-2 text-xs font-normal text-slate-500">disabled</span>
                    )}
                  </p>
                  <p className="mt-1 text-sm text-slate-600">
                    {searchName(scout.saved_search_id)} · score ≥ {scout.min_match_score} · up to{" "}
                    {scout.max_applications_per_run} per run ·{" "}
                    {scout.source === "local" ? "saved jobs" : "fetches from sources"}
                  </p>
                  {scout.last_run_at && (
                    <p className="mt-1 text-xs text-slate-500">
                      Last run {new Date(scout.last_run_at).toLocaleString()}
                    </p>
                  )}
                </div>
                <div className="flex shrink-0 gap-2">
                  <Button
                    onClick={() => runScout.mutate(scout.id)}
                    disabled={runScout.isPending || !scout.is_enabled}
                  >
                    {runScout.isPending ? "Running…" : "Run now"}
                  </Button>
                  <Button variant="secondary" onClick={() => startEditing(scout)}>
                    Edit
                  </Button>
                  <Button variant="destructive" onClick={() => deleteScout.mutate(scout.id)}>
                    Delete
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {report && <RunReport report={report} onDismiss={() => setReport(null)} />}

      {runs && runs.length > 0 && (
        <div className="mt-8">
          <h2 className="mb-2 font-medium">Recent runs</h2>
          <div className="space-y-2">
            {runs.map((run) => (
              <Card key={run.id}>
                <CardContent>
                  <button
                    type="button"
                    className="flex w-full items-center justify-between text-left"
                    onClick={() => setOpenRunId(openRunId === run.id ? null : run.id)}
                  >
                    <span className="text-sm">
                      {new Date(run.started_at).toLocaleString()} · {run.status}
                    </span>
                    <span className="text-xs text-slate-500">
                      {openRunId === run.id ? "Hide" : "What it did"}
                    </span>
                  </button>
                  {run.error && <p className="mt-1 text-sm text-red-600">{run.error}</p>}
                  {openRunId === run.id && (
                    <>
                      <RunTrail runId={run.id} />
                      <Link
                        to={`/activity/${run.id}`}
                        className="mt-2 inline-block text-xs text-slate-500 underline"
                      >
                        Open in Activity
                      </Link>
                    </>
                  )}
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

/// RunReport shows what a run just did, including why each job was
/// rejected — the run's reasoning is the point, not just its output.
function RunReport({ report, onDismiss }: { report: ScoutRunReport; onDismiss: () => void }) {
  const verdicts = report.verdicts ?? [];
  return (
    <Card className="mt-4">
      <CardContent>
        <div className="flex items-start justify-between">
          <div>
            <h2 className="font-medium">
              Considered {report.considered} job{report.considered === 1 ? "" : "s"}, created{" "}
              {report.created} application{report.created === 1 ? "" : "s"}
            </h2>
            {report.ceiling_hit && (
              <p className="mt-1 text-sm text-slate-600">
                Stopped at this scout's per-run limit. Run it again for more.
              </p>
            )}
            {report.error && <p className="mt-1 text-sm text-red-600">{report.error}</p>}
          </div>
          <Button variant="secondary" onClick={onDismiss}>
            Dismiss
          </Button>
        </div>

        {verdicts.length > 0 && (
          <ul className="mt-3 space-y-1 text-sm">
            {verdicts.map((verdict) => (
              <li key={verdict.job_id} className="flex gap-2">
                <span className={verdict.eligible ? "text-green-600" : "text-slate-400"}>
                  {verdict.eligible ? "✓" : "–"}
                </span>
                <span className="min-w-0">
                  <span className="font-medium">{verdict.job_title}</span>
                  <span className="text-slate-600"> — {verdict.reason}</span>
                </span>
              </li>
            ))}
          </ul>
        )}

        {report.created > 0 && (
          <p className="mt-3 text-sm">
            <Link className="underline" to="/applications">
              Review them
            </Link>{" "}
            — nothing has been sent.
          </p>
        )}
      </CardContent>
    </Card>
  );
}

function RunTrail({ runId }: { runId: string }) {
  const { data: events } = useScoutRunEvents(runId);
  if (!events || events.length === 0) {
    return <p className="mt-2 text-xs text-slate-500">No steps recorded.</p>;
  }
  return (
    <ul className="mt-2 space-y-1 text-xs text-slate-600">
      {events.map((event) => (
        <li key={event.id}>
          <span className="font-mono">{event.action}</span>
          {event.input_summary ? `: ${event.input_summary}` : ""}
          {event.output_summary ? ` — ${event.output_summary}` : ""}
        </li>
      ))}
    </ul>
  );
}
