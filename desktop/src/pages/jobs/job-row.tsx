import { cn } from "cn";
import { EyeOff, Undo2 } from "lucide-react";
import { ListItem } from "../../components/list-pane";
import { Button } from "../../components/ui/button";
import {
  hoursShiftFor,
  hoursWindow,
  jobLocation,
  jobSource,
  relativeTime,
  remoteLabel,
  scoreTone,
  workableLabel,
} from "../../lib/format";
import { useApplications } from "../../lib/queries/application.queries";
import { useSetJobInterest } from "../../lib/queries/job.queries";
import { useCandidateId, useProfile } from "../../lib/queries/profile.queries";
import { useCountries } from "../../lib/queries/saved-search.queries";
import { useToast } from "../../lib/toast";
import type { Job } from "../../lib/types";

export function JobRow({ job, score }: { job: Job; score?: number }) {
  // Where the job can be done from, as the daemon read it, reads better than
  // the source's own words — "Munich" for "München (81671)" — and the
  // source's words stand in when the daemon could not read them.
  // A job whose own words contradict its source's location says so, rather
  // than repeating the claim its page questions.
  const place = workableLabel(job) ?? (job.workable?.note ? "Location unclear" : jobLocation(job));
  const where = [remoteLabel(job.remote_type), place].filter(Boolean).join(" · ");
  const age = relativeTime(job.posted_at) ?? relativeTime(job.discovered_at);
  // The row is a link, which cannot hold a button, so the quick action sits
  // beside it and shows itself while the row is hovered or focused.
  return (
    <div className="group relative">
      <JobRowLink job={job} score={score} where={where} age={age} />
      <RowInterestButton job={job} />
    </div>
  );
}

/// RowInterestButton rules a job out, or back in, from the list. Ruling one
/// out leaves its applications as they are, which the notice says.
function RowInterestButton({ job }: { job: Job }) {
  const { notify, notifyError } = useToast();
  const { candidateId } = useCandidateId();
  const { data: applications } = useApplications(candidateId);
  const ruledOut = !!job.not_interested_at;
  const setInterest = useSetJobInterest({
    onSuccess: (updated) => {
      if (!updated.not_interested_at) {
        notify(`“${updated.title}” is back in your lists.`, "success");
        return;
      }
      const kept = applications?.filter((a) => a.job_id === updated.id).length ?? 0;
      notify(
        `“${updated.title}” marked not interested.` +
          (kept === 0
            ? ""
            : kept === 1
              ? " Its application is unchanged."
              : ` Its ${kept} applications are unchanged.`),
        "success",
      );
    },
    onError: notifyError,
  });
  const label = ruledOut ? "Interested again" : "Not interested";
  return (
    <Button
      size="icon-xs"
      variant="outline"
      aria-label={`${label}: ${job.title}`}
      title={label}
      disabled={setInterest.isPending}
      onClick={() => setInterest.mutate({ id: job.id, notInterested: !ruledOut })}
      className="absolute right-3 bottom-2 bg-popover opacity-0 shadow-xs transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
    >
      {ruledOut ? <Undo2 /> : <EyeOff />}
    </Button>
  );
}

function JobRowLink({ job, score, where, age }: { job: Job; score?: number; where: string; age?: string }) {
  return (
    <ListItem to={`/jobs/${job.id}`}>
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="line-clamp-2 text-[13px] leading-snug font-medium">{job.title}</p>
          <p className="mt-0.5 truncate text-xs text-subtle-foreground">{job.company_name || "Unknown company"}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {[where, jobSource(job, true), age].filter(Boolean).join(" · ")}
          </p>
          <JobMarks job={job} />
        </div>
        {score !== undefined && (
          <span
            title="Match score"
            className={`mt-0.5 shrink-0 rounded-md px-1.5 py-0.5 text-xs font-semibold tabular-nums ${scoreTone(score)}`}
          >
            {score}
          </span>
        )}
      </div>
    </ListItem>
  );
}

/// JobMarks are what a row says beyond where: help moving, which is worth
/// seeing at a glance, and working hours a job requires, marked by whether
/// the candidate's time zone is inside them (ADR-0025).
function JobMarks({ job }: { job: Job }) {
  const { data: countries } = useCountries();
  const { data: profile } = useProfile();
  const offers = [
    job.relocation?.status === "offered" && "Relocation",
    job.visa_sponsorship?.status === "offered" && "Visa sponsorship",
  ].filter(Boolean) as string[];
  const hours = job.hours?.status === "required" ? job.hours : undefined;
  const zone = countries?.time_zones.find((z) => z.id === profile?.time_zone);
  const fits = hours && zone ? hoursShiftFor(hours, zone.offset) === 0 : undefined;
  if (offers.length === 0 && !hours) return null;
  return (
    <p className="mt-1 flex flex-wrap gap-1">
      {offers.map((offer) => (
        <span key={offer} className="rounded bg-success/12 px-1.5 py-px text-[11px] font-medium text-success">
          {offer}
        </span>
      ))}
      {hours && (
        <span
          title={hours.evidence}
          className={cn(
            "rounded px-1.5 py-px text-[11px] font-medium",
            fits === undefined
              ? "bg-muted text-subtle-foreground"
              : fits
                ? "bg-success/12 text-success"
                : "bg-warning/14 text-warning",
          )}
        >
          Hours {hoursWindow(hours)}
        </span>
      )}
    </p>
  );
}
