import { Building2, Check } from "lucide-react";
import { Link } from "react-router-dom";
import { Button } from "../../components/ui/button";
import {
  dateTime,
  employmentLabel,
  jobLocation,
  jobSource,
  relativeTime,
  remoteLabel,
  salaryLabel,
} from "../../lib/format";
import { useJobCompany, useWatchCompany } from "../../lib/queries/company.queries";
import { useToast } from "../../lib/toast";
import type { Job } from "../../lib/types";

export function JobHeading({ job }: { job: Job }) {
  // A location of "Remote" says what the remote type already does.
  const facts = [
    ...new Set(
      [remoteLabel(job.remote_type), employmentLabel(job.employment_type), jobLocation(job), salaryLabel(job)].filter(
        (fact): fact is string => !!fact,
      ),
    ),
  ];
  const posted = relativeTime(job.posted_at);

  return (
    <header>
      <h2 className="text-xl leading-tight font-semibold select-text">{job.title}</h2>
      <div className="mt-1 flex min-h-6 items-center gap-2">
        <p className="text-sm text-subtle-foreground select-text">{job.company_name || "Unknown company"}</p>
        <WatchCompany job={job} />
      </div>
      {facts.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {facts.map((fact) => (
            <span key={fact} className="rounded-md border border-border px-2 py-0.5 text-xs text-subtle-foreground">
              {fact}
            </span>
          ))}
        </div>
      )}
      <p className="mt-3 text-xs text-muted-foreground">
        From {jobSource(job)}
        {posted && <span title={dateTime(job.posted_at)}> · posted {posted}</span>}
        {!posted && relativeTime(job.discovered_at) && (
          <span title={dateTime(job.discovered_at)}> · found {relativeTime(job.discovered_at)}</span>
        )}
      </p>
    </header>
  );
}

/// WatchCompany offers to watch the company whose hiring page the posting
/// is on, so its later openings reach the candidate's searches
/// (docs/02_features/17_company_watchlists.md AC-002). A job on no page
/// JobPilot reads offers nothing, and a company already watched says so.
function WatchCompany({ job }: { job: Job }) {
  const { notify } = useToast();
  const { data } = useJobCompany(job.id);
  const watch = useWatchCompany({
    onSuccess: (company) =>
      notify(`Watching ${company.name}. Your searches now look at its hiring page too.`, "success"),
    onError: (err) => notify(`Couldn’t watch this company: ${err.message}`),
  });

  if (!data?.board) return null;
  if (data.company) {
    return (
      <Link
        to="/settings?section=searches"
        className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        title={`Its hiring page, ${data.company.url}, is read by your searches`}
      >
        <Check className="size-3" />
        Watching
      </Link>
    );
  }
  return (
    <Button
      size="xs"
      variant="ghost"
      className="text-muted-foreground"
      disabled={watch.isPending}
      title={`Find its openings on ${data.url}`}
      onClick={() => watch.mutate({ job_id: job.id })}
    >
      <Building2 data-icon="inline-start" />
      {watch.isPending ? "Checking its page…" : "Watch this company"}
    </Button>
  );
}
