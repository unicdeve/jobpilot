import { Link } from "react-router-dom";
import { Panel, Section } from "../../components/page";
import { StatusBadge } from "../../components/status-badge";
import { Button } from "../../components/ui/button";
import type { ScoutRunReport } from "../../lib/types";

/// RunReport shows what a run just did, including why each job was
/// rejected — the run's reasoning is the point, not just its output.
export function RunReport({ report, onDismiss }: { report: ScoutRunReport; onDismiss: () => void }) {
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
