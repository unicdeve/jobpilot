import { openUrl } from "@tauri-apps/plugin-opener";
import { ArrowLeft, CircleAlert, CircleCheck, ExternalLink, Sparkles } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { FormField } from "../components/form-field";
import { Markdown } from "../components/markdown";
import { DetailSection, Toolbar } from "../components/page";
import { splitBackClass } from "../components/split-view";
import { StatusBadge } from "../components/status-badge";
import { ErrorLine, Loading } from "../components/status-line";
import { Button } from "../components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import {
  dateTime,
  employmentLabel,
  jobLocation,
  relativeTime,
  remoteLabel,
  salaryLabel,
  scoreTone,
  sourceLabel,
} from "../lib/format";
import {
  useAnalyzeMatch,
  useApplications,
  useCandidateId,
  useCreateApplication,
  useJob,
  useMatches,
  useResumes,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type { Application, ApplicationMode, Job, JobMatch } from "../lib/types";

/// See docs/00_product/04_application_modes.md. Neither mode submits.
const MODE_LABELS: Record<ApplicationMode, string> = {
  review: "Review — fills in the form for you to check and submit",
  assist: "Assist — writes the content, never touches the form",
};

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
  const { data: matches } = useMatches(candidateId);
  const { data: applications } = useApplications(candidateId);

  const match = matches?.find((m) => m.job_id === id);
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
            {candidateId && <StartApplicationButton candidateId={candidateId} jobId={job.id} again={existing.length > 0} />}
          </>
        }
      />

      <article className="max-w-3xl px-8 pt-6 pb-12">
        <JobHeading job={job} />

        {existing.length > 0 && <ExistingApplications applications={existing} />}

        <MatchSection candidateId={candidateId} match={match} />

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

function JobHeading({ job }: { job: Job }) {
  const facts = [
    remoteLabel(job.remote_type),
    employmentLabel(job.employment_type),
    jobLocation(job),
    salaryLabel(job),
  ].filter(Boolean) as string[];
  const posted = relativeTime(job.posted_at);

  return (
    <header>
      <h2 className="text-xl leading-tight font-semibold select-text">{job.title}</h2>
      <p className="mt-1 text-sm text-subtle-foreground select-text">{job.company_name || "Unknown company"}</p>
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
        From {sourceLabel(job.source_id)}
        {posted && <span title={dateTime(job.posted_at)}> · posted {posted}</span>}
        {!posted && relativeTime(job.discovered_at) && (
          <span title={dateTime(job.discovered_at)}> · found {relativeTime(job.discovered_at)}</span>
        )}
      </p>
    </header>
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

function MatchSection({ candidateId, match }: { candidateId: string | null; match: JobMatch | undefined }) {
  if (!candidateId) {
    return (
      <DetailSection title="Match">
        <p className="text-sm text-muted-foreground">
          Matching compares this job with your profile.{" "}
          <Link to="/profile" className="text-foreground underline">
            Create a profile
          </Link>{" "}
          to analyze it.
        </p>
      </DetailSection>
    );
  }
  if (!match) {
    return (
      <DetailSection title="Match">
        <p className="text-sm text-muted-foreground">
          Not analyzed yet. Analyze match compares the posting with your profile and explains the fit.
        </p>
      </DetailSection>
    );
  }

  return (
    <DetailSection title="Match">
      <div className="flex items-start gap-4">
        <div className={`flex size-14 shrink-0 flex-col items-center justify-center rounded-xl ${scoreTone(match.score)}`}>
          <span className="text-xl leading-none font-semibold tabular-nums">{match.score}</span>
          <span className="mt-0.5 text-[10px] font-medium opacity-80">of 100</span>
        </div>
        <div className="min-w-0">
          <p className="text-sm leading-relaxed text-subtle-foreground select-text">{match.rationale}</p>
          <p className="mt-1.5 text-xs text-muted-foreground">
            Confidence {Math.round(match.confidence * 100)}% · analyzed {relativeTime(match.created_at) ?? "recently"}
            {match.model_name && ` with ${match.model_name}`}
          </p>
        </div>
      </div>
      <div className="mt-5 grid gap-5 sm:grid-cols-2">
        <FindingList title="Strengths" items={match.strengths} icon={<CircleCheck className="text-success" />} />
        <FindingList title="Gaps" items={match.gaps} icon={<CircleAlert className="text-warning" />} />
      </div>
    </DetailSection>
  );
}

function FindingList({ title, items, icon }: { title: string; items: string[] | undefined; icon: ReactNode }) {
  if (!items?.length) return null;
  return (
    <div>
      <p className="mb-2 text-xs font-semibold text-subtle-foreground">{title}</p>
      <ul className="space-y-2">
        {items.map((item, i) => (
          <li key={i} className="flex gap-2 text-[13px] leading-snug text-subtle-foreground select-text">
            <span className="mt-0.5 shrink-0 [&_svg]:size-3.5">{icon}</span>
            {item}
          </li>
        ))}
      </ul>
    </div>
  );
}

function AnalyzeButton({ candidateId, jobId, analyzed }: { candidateId: string; jobId: string; analyzed: boolean }) {
  const { notify } = useToast();
  const analyzeMatch = useAnalyzeMatch({ onError: (err) => notify(String(err)) });
  return (
    <Button
      size="sm"
      variant="outline"
      onClick={() => analyzeMatch.mutate({ candidateId, jobId })}
      disabled={analyzeMatch.isPending}
    >
      <Sparkles data-icon="inline-start" />
      {analyzeMatch.isPending ? "Analyzing…" : analyzed ? "Re-analyze" : "Analyze match"}
    </Button>
  );
}

/// StartApplicationButton asks the two things an application needs — which
/// resume, and how involved JobPilot should be — in a dialog, so the job's
/// page is about the job rather than a form.
function StartApplicationButton({ candidateId, jobId, again }: { candidateId: string; jobId: string; again: boolean }) {
  const { notify } = useToast();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const { data: resumes } = useResumes(candidateId);
  // null rather than "" so the Select shows its placeholder: a resume is
  // optional on an application.
  const [resumeId, setResumeId] = useState<string | null>(null);
  const [mode, setMode] = useState<ApplicationMode>("review");

  // Base UI's Select renders the raw value in the trigger unless it is told
  // how values map to labels — which for a resume would mean showing its
  // UUID, the exact thing this screen exists to stop the user handling.
  const resumeLabels = Object.fromEntries((resumes ?? []).map((r) => [r.id, r.name]));

  const createApplication = useCreateApplication({
    // Going straight to the new application is the point of starting one
    // here: the next thing the user wants is its cover letter and questions.
    onSuccess: (application) => navigate(`/applications/${application.id}`),
    onError: (err) => notify(String(err)),
  });

  const create = (event: React.FormEvent) => {
    event.preventDefault();
    createApplication.mutate({ jobId, candidateId, resumeId: resumeId ?? "", mode });
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button size="sm" onClick={() => setOpen(true)}>
        {again ? "Apply again" : "Start application"}
      </Button>
      <DialogContent>
        <form onSubmit={create}>
          <DialogTitle>{again ? "Start another application" : "Start an application"}</DialogTitle>
          <DialogDescription>
            JobPilot prepares the application for you to review. Nothing is sent to the employer — you submit it
            yourself.
          </DialogDescription>
          <div className="mt-4 flex flex-col gap-3">
            <FormField label="Resume" htmlFor="resume">
              <Select items={resumeLabels} value={resumeId} onValueChange={(value) => setResumeId(value)}>
                <SelectTrigger id="resume" className="w-full">
                  <SelectValue placeholder={resumes?.length ? "None" : "No resumes imported"} />
                </SelectTrigger>
                <SelectContent>
                  {resumes?.map((resume) => (
                    <SelectItem key={resume.id} value={resume.id}>
                      {resume.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
            <FormField label="Mode" htmlFor="mode">
              <Select items={MODE_LABELS} value={mode} onValueChange={(value) => setMode(value as ApplicationMode)}>
                <SelectTrigger id="mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(MODE_LABELS).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
          </div>
          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>Cancel</DialogClose>
            <Button type="submit" disabled={createApplication.isPending}>
              {createApplication.isPending ? "Creating…" : "Create application"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
