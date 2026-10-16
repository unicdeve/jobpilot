import { openUrl } from "@tauri-apps/plugin-opener";
import { ArrowLeft, Briefcase, Ellipsis, ExternalLink, EyeOff, Undo2 } from "lucide-react";
import { useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { DeletionDialog } from "../../components/deletion-dialog";
import { RecordGone } from "../../components/list-pane";
import { Markdown } from "../../components/markdown";
import { DetailSection, Toolbar } from "../../components/page";
import { splitBackClass } from "../../components/split-view";
import { StatusBadge } from "../../components/status-badge";
import { ErrorLine, Loading } from "../../components/status-line";
import { Button } from "../../components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import { isNotFound } from "../../lib/api";
import { dateTime, relativeTime } from "../../lib/format";
import { useApplications } from "../../lib/queries/application.queries";
import { useJob, useSetJobInterest } from "../../lib/queries/job.queries";
import { useJobMatch } from "../../lib/queries/match.queries";
import { useCandidateId } from "../../lib/queries/profile.queries";
import { useToast } from "../../lib/toast";
import type { Application, Job } from "../../lib/types";
import { JobHeading } from "./job-heading";
import { AnalyzeButton, MatchSection } from "./match-section";
import { StartApplicationButton } from "./start-application-button";
import { WhereSection } from "./where-section";

/// JobDetail is the hub the rest of the app navigates through: everything
/// the user can do with a job happens here, rather than by copying its ID to
/// another screen. See docs/00_product/03_user_flows.md — "user opens job →
/// loads candidate context → match score persisted", and "user selects job →
/// system generates cover letter".
export function JobDetail() {
  const { id } = useParams<{ id: string }>();
  const { search } = useLocation();
  const { candidateId } = useCandidateId();
  const { data: job, error, isPending } = useJob(id);
  const { data: match } = useJobMatch(id);
  const { data: applications } = useApplications(candidateId);

  const existing = applications?.filter((a) => a.job_id === id) ?? [];

  const back = (
    <Button
      size="icon-sm"
      variant="ghost"
      className={splitBackClass}
      aria-label="Back to jobs"
      render={<Link to={{ pathname: "/jobs", search }} />}
    >
      <ArrowLeft />
    </Button>
  );

  if (isNotFound(error)) {
    return (
      <RecordGone
        back={back}
        icon={<Briefcase />}
        title="This job no longer exists"
        description="It may have been deleted. Choose another job from the list."
      />
    );
  }
  if (error) return <ErrorLine error={error} />;
  if (isPending || !job) return <Loading />;

  return (
    <div className="min-h-full">
      <Toolbar
        leading={back}
        className="px-5"
        actions={
          <>
            {job.canonical_url && (
              <Button size="sm" variant="ghost" onClick={() => void openUrl(job.canonical_url)}>
                <ExternalLink data-icon="inline-start" />
                Open posting
              </Button>
            )}
            {candidateId && <AnalyzeButton candidateId={candidateId} jobId={job.id} analyzed={!!match} />}
            {candidateId && (
              <StartApplicationButton candidateId={candidateId} jobId={job.id} again={existing.length > 0} />
            )}
            <InterestButton job={job} applications={existing.length} />
            <JobMenu job={job} />
          </>
        }
      />

      <article className="max-w-3xl px-8 pt-6 pb-12">
        {job.not_interested_at && <RuledOutBanner job={job} />}
        <JobHeading job={job} />

        {existing.length > 0 && <ExistingApplications applications={existing} />}

        <WhereSection job={job} />

        <MatchSection candidateId={candidateId} jobId={job.id} match={match ?? undefined} />

        {job.skills?.length > 0 && (
          <DetailSection title="Skills">
            <div className="flex flex-wrap gap-1.5">
              {job.skills.map((skill) => (
                <span key={skill} className="rounded-md bg-muted px-2 py-0.5 text-xs text-subtle-foreground">
                  {skill}
                </span>
              ))}
            </div>
          </DetailSection>
        )}

        <DetailSection title="Description">
          {job.description_markdown ? (
            <Markdown>{job.description_markdown}</Markdown>
          ) : (
            <p className="text-sm text-muted-foreground">No description was captured for this job.</p>
          )}
        </DetailSection>
      </article>
    </div>
  );
}

/// InterestButton rules the job out, or back in. Neither asks: both are
/// reversible, and ruling a job out changes nothing but where it is shown —
/// its applications are left as they are, which the notice says.
function InterestButton({ job, applications }: { job: Job; applications: number }) {
  const { notify, notifyError } = useToast();
  const setInterest = useSetJobInterest({
    onSuccess: (updated) => {
      if (!updated.not_interested_at) {
        notify("Back in your lists.", "success");
        return;
      }
      notify(
        applications > 0
          ? `Marked not interested. Its ${applications === 1 ? "application is" : `${applications} applications are`} unchanged.`
          : "Marked not interested. It is hidden from your lists, and scouts pass it over.",
        "success",
      );
    },
    onError: notifyError,
  });
  const ruledOut = !!job.not_interested_at;
  return (
    <Button
      size="sm"
      variant="ghost"
      disabled={setInterest.isPending}
      onClick={() => setInterest.mutate({ id: job.id, notInterested: !ruledOut })}
    >
      {ruledOut ? <Undo2 data-icon="inline-start" /> : <EyeOff data-icon="inline-start" />}
      {ruledOut ? "Interested again" : "Not interested"}
    </Button>
  );
}

/// JobMenu holds what is done to the job rather than with it.
function JobMenu({ job }: { job: Job }) {
  const { notify } = useToast();
  const navigate = useNavigate();
  const { search } = useLocation();
  const [deleting, setDeleting] = useState(false);
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button size="icon-sm" variant="ghost" aria-label="More actions" title="More actions" />}
        >
          <Ellipsis />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem variant="destructive" onClick={() => setDeleting(true)}>
            Delete job…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DeletionDialog
        target="job"
        id={job.id}
        open={deleting}
        onOpenChange={setDeleting}
        title={`Delete “${job.title}”?`}
        note={
          job.not_interested_at
            ? "It is marked not interested; deleting it forgets that, so a search may find it again."
            : "A later search may find it again. To keep it out of your lists and scouts, mark it not interested instead."
        }
        confirmLabel="Delete job"
        onDeleted={() => {
          notify("Job deleted.", "success");
          navigate({ pathname: "/jobs", search });
        }}
      />
    </>
  );
}

/// RuledOutBanner says why a job that left every list is still here: it was
/// opened directly, from an application or a link.
function RuledOutBanner({ job }: { job: Job }) {
  return (
    <p className="mb-5 flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-[13px] text-subtle-foreground">
      <EyeOff className="size-4 shrink-0 text-muted-foreground" />
      <span>
        You marked this job not interested{" "}
        <span title={dateTime(job.not_interested_at)}>{relativeTime(job.not_interested_at)}</span>. It is hidden from
        your lists, and scouts pass it over.
      </span>
    </p>
  );
}

function ExistingApplications({ applications }: { applications: Application[] }) {
  return (
    <div className="mt-5 flex flex-col gap-1.5">
      {applications.map((app) => (
        <Link
          key={app.id}
          to={`/applications/${app.id}`}
          className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card px-3 py-2 text-sm hover:bg-accent/60"
        >
          <span>
            {app.mode === "assist" ? "Assist" : "Review"} application
            <span className="text-muted-foreground"> · updated {relativeTime(app.updated_at) ?? "recently"}</span>
          </span>
          <StatusBadge value={app.state} />
        </Link>
      ))}
    </div>
  );
}
