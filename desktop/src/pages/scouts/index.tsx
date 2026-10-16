import { Plus, Radar } from "lucide-react";
import { useState } from "react";
import { ConfirmDialog } from "../../components/confirm";
import { EmptyState } from "../../components/empty-state";
import { Page, Panel, Section } from "../../components/page";
import { Button } from "../../components/ui/button";
import { useSavedSearches } from "../../lib/queries/saved-search.queries";
import { useDeleteScout, useRunScout, useScoutRuns, useScouts } from "../../lib/queries/scout.queries";
import { useToast } from "../../lib/toast";
import type { Scout, ScoutRunReport } from "../../lib/types";
import { RecentRuns } from "./recent-runs";
import { RunReport } from "./run-report";
import { ScoutEditor } from "./scout-editor";
import { ScoutRow } from "./scout-row";

export function Scouts() {
  const { notify, notifyError } = useToast();
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
    onError: notifyError,
  });
  const { data: scouts } = useScouts(runScout.isPending);
  const { data: searches } = useSavedSearches();
  const { data: runs } = useScoutRuns(10);

  // editing is the scout being edited, or "new"; null when the editor is shut.
  const [editing, setEditing] = useState<Scout | "new" | null>(null);
  const [deleting, setDeleting] = useState<Scout | null>(null);

  const deleteScout = useDeleteScout({
    onSuccess: () => setDeleting(null),
    onError: notifyError,
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
            {scouts.map((scout) => (
              <ScoutRow
                key={scout.id}
                scout={scout}
                searchName={searchName(scout.saved_search_id)}
                asked={runScout.isPending && runScout.variables === scout.id}
                runDisabled={runScout.isPending}
                onRun={() => runScout.mutate(scout.id)}
                onEdit={() => setEditing(scout)}
                onDelete={() => setDeleting(scout)}
              />
            ))}
          </Panel>
        )}
      </Section>

      {report && <RunReport report={report} onDismiss={() => setReport(null)} />}

      {runs && runs.length > 0 && <RecentRuns runs={runs} scoutName={scoutName} />}

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
