import { openUrl } from "@tauri-apps/plugin-opener";
import { ChevronRight, ExternalLink, Globe } from "lucide-react";
import { type FormEvent, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { DetailSection } from "../../components/page";
import { StatusBadge } from "../../components/status-badge";
import { Thinking } from "../../components/thinking";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { runDuration } from "../../lib/activity";
import { dateTime, relativeTime } from "../../lib/format";
import { usePrepareInBrowser, useSetApplyUrl, useStopAgentRun } from "../../lib/queries/application.queries";
import { useToast } from "../../lib/toast";
import { type AgentRun, type AgentRunWithEvents, type Application, isRunInProgress, type Job } from "../../lib/types";

export function BrowserSection({
  application,
  job,
  runs,
}: {
  application: Application;
  job: Job | undefined;
  runs: AgentRunWithEvents[];
}) {
  const { notifyError } = useToast();
  const prepareInBrowser = usePrepareInBrowser({ onError: notifyError });
  const stopRun = useStopAgentRun({ onError: notifyError });

  // Runs are listed newest first, and only the newest can be in progress.
  const latest = runs[0]?.run;
  const inProgress = latest !== undefined && isRunInProgress(latest.status);
  useAnnounceFinished(latest);

  const prepare = () => prepareInBrowser.mutate(application.id);
  const stop = latest && inProgress ? () => stopRun.mutate(latest.id) : undefined;

  return (
    <DetailSection
      title="Prepare in browser"
      description="Opens the application form in Chrome and fills in what JobPilot knows. The window stays open for you to check and submit — JobPilot never submits."
      actions={
        <Button size="sm" variant="outline" onClick={prepare} disabled={prepareInBrowser.isPending || inProgress}>
          <Globe data-icon="inline-start" />
          {prepareInBrowser.isPending || inProgress ? "Preparing…" : runs.length > 0 ? "Prepare again" : "Prepare"}
        </Button>
      }
    >
      {application.apply_url && !inProgress && <StartsFrom application={application} />}

      {latest?.status === "running" && (
        <div className="mb-3 flex items-center justify-between gap-3">
          <Thinking state="working">Preparing in the browser…</Thinking>
          <Button size="sm" variant="ghost" onClick={stop} disabled={stopRun.isPending}>
            Stop
          </Button>
        </div>
      )}

      {latest?.status === "waiting_for_user" && (
        <div role="status" className="mb-3 rounded-lg border border-warning/35 bg-warning/8 px-4 py-3.5">
          <p className="text-sm font-medium">Answer the check in the browser window</p>
          <p className="mt-0.5 text-sm text-muted-foreground">
            The page wants to know you're a person. Answer it in the JobPilot window, and preparing carries on by
            itself. If it keeps asking, stop here and go through your own browser instead.
          </p>
          <Button size="sm" variant="outline" className="mt-3" onClick={stop} disabled={stopRun.isPending}>
            Stop
          </Button>
        </div>
      )}

      {latest?.status === "blocked" && (
        <HandOff
          application={application}
          postingUrl={application.apply_url || job?.canonical_url}
          onPrepare={prepare}
          preparing={prepareInBrowser.isPending}
        />
      )}

      {runs.length > 0 && (
        <ul className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-card">
          {runs.map(({ run, events }) => {
            const duration = runDuration(run);
            return (
              <li key={run.id}>
                <Link to={`/activity/${run.id}`} className="flex items-start gap-3 px-4 py-2.5 hover:bg-accent/60">
                  <div className="min-w-0 flex-1">
                    <p className="text-[13px]">
                      <span title={dateTime(run.started_at)}>{relativeTime(run.started_at)}</span>
                      <span className="text-muted-foreground">
                        {duration && <> · took {duration}</>} · {events.length} {events.length === 1 ? "step" : "steps"}
                      </span>
                    </p>
                    {run.error && <p className="mt-0.5 text-[13px] text-destructive">{run.error}</p>}
                  </div>
                  <StatusBadge value={run.status} className="shrink-0" />
                  <ChevronRight className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </DetailSection>
  );
}

/// useAnnounceFinished tells the user when a run they watched start has
/// filled in the form. A run that stops says why in the list and offers the
/// way round, so only success needs saying elsewhere.
function useAnnounceFinished(latest: AgentRun | undefined) {
  const { notify } = useToast();
  const watched = useRef<{ id: string; inProgress: boolean } | undefined>(undefined);
  useEffect(() => {
    if (!latest) return;
    const previous = watched.current;
    watched.current = { id: latest.id, inProgress: isRunInProgress(latest.status) };
    if (previous?.id === latest.id && previous.inProgress && latest.status === "completed") {
      notify("Browser window filled in — review it there before submitting.", "success");
    }
  }, [latest, notify]);
}

/// StartsFrom says a run begins at the link the user gave rather than the
/// job's own, and lets them go back to the job's.
function StartsFrom({ application }: { application: Application }) {
  const { notifyError } = useToast();
  const setApplyUrl = useSetApplyUrl({ onError: notifyError });
  return (
    <p className="mb-3 text-[13px] text-muted-foreground">
      Starts from <span className="break-all text-foreground select-text">{application.apply_url}</span>
      {" · "}
      <Button
        size="xs"
        variant="link"
        className="h-auto px-0 text-[13px]"
        onClick={() => setApplyUrl.mutate({ applicationId: application.id, url: "" })}
        disabled={setApplyUrl.isPending}
      >
        Use the job's own link
      </Button>
    </p>
  );
}

/// HandOff is the way round a run that stopped: the user takes the step
/// JobPilot's browser could not, in their own browser where a site's check
/// passes, and gives back the address of the form it led to (ADR-0033 §4).
function HandOff({
  application,
  postingUrl,
  onPrepare,
  preparing,
}: {
  application: Application;
  postingUrl: string | undefined;
  onPrepare: () => void;
  preparing: boolean;
}) {
  const { notifyError } = useToast();
  const [formUrl, setFormUrl] = useState("");
  const setApplyUrl = useSetApplyUrl({
    onSuccess: () => {
      setFormUrl("");
      onPrepare();
    },
    onError: notifyError,
  });

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setApplyUrl.mutate({ applicationId: application.id, url: formUrl.trim() });
  };

  return (
    <div className="mb-3 rounded-lg border border-border bg-muted/50 px-4 py-3.5">
      <p className="text-sm font-medium">Get to the form in your own browser</p>
      <p className="mt-0.5 text-sm text-muted-foreground">
        Open the posting in your usual browser, follow its apply button, and paste the address of the application form
        here. JobPilot fills it in from there.
      </p>
      {postingUrl && (
        <Button size="sm" variant="outline" className="mt-3" onClick={() => void openUrl(postingUrl)}>
          <ExternalLink data-icon="inline-start" />
          Open the posting
        </Button>
      )}
      <form className="mt-3 flex gap-2" onSubmit={submit}>
        <Input
          type="url"
          value={formUrl}
          onChange={(event) => setFormUrl(event.target.value)}
          placeholder="https://jobs.example.com/…/application"
          aria-label="Application form address"
          className="bg-background"
        />
        <Button
          size="sm"
          type="submit"
          className="shrink-0"
          disabled={!formUrl.trim() || setApplyUrl.isPending || preparing}
        >
          Prepare from this link
        </Button>
      </form>
    </div>
  );
}
