import { ChevronRight, Globe } from "lucide-react";
import { Link } from "react-router-dom";
import { DetailSection } from "../../components/page";
import { StatusBadge } from "../../components/status-badge";
import { Button } from "../../components/ui/button";
import { runDuration } from "../../lib/activity";
import { dateTime, relativeTime } from "../../lib/format";
import { usePrepareInBrowser } from "../../lib/queries/application.queries";
import { useToast } from "../../lib/toast";
import type { AgentRunWithEvents } from "../../lib/types";

export function BrowserSection({ applicationId, runs }: { applicationId: string; runs: AgentRunWithEvents[] }) {
  const { notify, notifyError } = useToast();
  const prepareInBrowser = usePrepareInBrowser({
    onSuccess: () => notify("Browser window opened — review it there before submitting.", "success"),
    // A blocked or failed run is expected, structured output (a CAPTCHA, an
    // unreadable form), not just a transport error; its reason is also
    // listed below once the runs refresh, which they do on failure too.
    onError: notifyError,
  });

  return (
    <DetailSection
      title="Prepare in browser"
      description="Opens the application form in Chrome and fills in what JobPilot knows. The window stays open for you to check and submit — JobPilot never submits."
      actions={
        <Button
          size="sm"
          variant="outline"
          onClick={() => prepareInBrowser.mutate(applicationId)}
          disabled={prepareInBrowser.isPending}
        >
          <Globe data-icon="inline-start" />
          {prepareInBrowser.isPending ? "Preparing…" : runs.length > 0 ? "Prepare again" : "Prepare"}
        </Button>
      }
    >
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
