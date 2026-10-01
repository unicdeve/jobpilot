import { openUrl } from "@tauri-apps/plugin-opener";
import { ArrowLeft, Building2, Check, CircleAlert, CircleCheck, Ellipsis, EyeOff, ExternalLink, Sparkles, Undo2 } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { DeletionDialog } from "../components/deletion-dialog";
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../components/ui/dropdown-menu";
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
  offerLabel,
  placesLabel,
  relativeTime,
  remoteLabel,
  salaryLabel,
  scoreTone,
  jobSource,
  hoursShiftFor,
  hoursWindow,
} from "../lib/format";
import {
  useAnalyzeMatch,
  useApplications,
  useCandidateId,
  useCountries,
  useCreateApplication,
  useJob,
  useJobCompany,
  useJobMatch,
  useProfile,
  useResumes,
  useSetJobInterest,
  useWatchCompany,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type { Application, ApplicationMode, Job, JobMatch, WorkingHours } from "../lib/types";

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

        <MatchSection candidateId={candidateId} match={match ?? undefined} />

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
  const { notify } = useToast();
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
    onError: (err) => notify(String(err)),
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
        <DropdownMenuTrigger render={<Button size="icon-sm" variant="ghost" aria-label="More actions" title="More actions" />}>
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
        <span title={dateTime(job.not_interested_at)}>{relativeTime(job.not_interested_at)}</span>. It is hidden from your
        lists, and scouts pass it over.
      </span>
    </p>
  );
}

function JobHeading({ job }: { job: Job }) {
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

/// WhereSection says where the job can be done from and whether it helps a
/// candidate move, as the daemon read the posting (ADR-0023), with the
/// posting's own words beside each: a label read from text is only as good
/// as the sentence it came from, so the sentence is shown.
function WhereSection({ job }: { job: Job }) {
  const { data: countries } = useCountries();
  const home = countries?.countries.find((c) => c.code === countries.profile_country);
  const workable = job.workable;
  const places = workable?.places?.filter(Boolean) ?? [];

  let summary: ReactNode;
  let quote: string | undefined;
  switch (workable?.scope) {
    case "anywhere":
      summary = "Open to candidates anywhere.";
      break;
    case "listed":
      summary = `${job.remote_type === "remote" ? "Open in" : "Based in"} ${placesLabel(places)}.`;
      break;
    default:
      quote = workable?.note;
      summary = !quote
        ? "The posting doesn’t say where it can be done from."
        : quote === job.title
          ? `The listing on ${jobSource(job)} and the title disagree about where this can be done from:`
          : "It isn’t clear where this can be done from. The posting says:";
  }
  const openToYou =
    home && workable && (workable.scope === "anywhere" || (workable.countries ?? []).includes(home.code));
  const listed = job.locations?.filter(Boolean) ?? [];

  const offers = [
    { topic: "relocation" as const, offer: job.relocation },
    { topic: "visa" as const, offer: job.visa_sponsorship },
  ].filter(({ offer }) => offer?.status);

  return (
    <DetailSection title="Where you can work">
      <div className="space-y-3 text-[13px] leading-snug">
        <div>
          <p className="text-subtle-foreground select-text">
            {summary}
            {home && workable?.scope !== "unknown" && (
              <span className={openToYou ? "text-success" : "text-warning"}>
                {" "}
                {openToYou
                  ? `You can do it from ${home.name}.`
                  : job.remote_type === "remote"
                    ? `Not open in ${home.name}.`
                    : `You’d have to move from ${home.name}.`}
              </span>
            )}
          </p>
          {quote && <Evidence>{quote}</Evidence>}
          {workable?.scope !== "anywhere" && listed.length > 0 && (
            <SourceLocations source={capitalize(jobSource(job))} locations={listed} />
          )}
        </div>
        {job.hours?.status && <HoursLine hours={job.hours} />}
        {offers.map(({ topic, offer }) => (
          <div key={topic}>
            <p
              className={
                offer.status === "offered"
                  ? "font-medium text-success"
                  : offer.status === "declined"
                    ? "font-medium text-subtle-foreground"
                    : "font-medium text-foreground"
              }
            >
              {offerLabel(topic, offer.status)}
            </p>
            {offer.evidence && <Evidence>{offer.evidence}</Evidence>}
          </div>
        ))}
      </div>
    </DetailSection>
  );
}

/// HoursLine is the working hours a job asks for, against the candidate's
/// own time zone when the profile has one (ADR-0025). Where and when stay
/// separate lines: an anywhere job asking for CET hours is both.
function HoursLine({ hours }: { hours: WorkingHours }) {
  const { data: countries } = useCountries();
  const { data: profile } = useProfile();
  const zone = countries?.time_zones.find((z) => z.id === profile?.time_zone);
  const shift = zone ? hoursShiftFor(hours, zone.offset) : undefined;
  const required = hours.status === "required";

  return (
    <div>
      <p className="font-medium text-foreground">
        {required ? "Hours required" : "Hours preferred"}: {hoursWindow(hours)}
        {shift !== undefined && (
          <span className={shift === 0 ? "font-normal text-success" : "font-normal text-warning"}>
            {" "}
            {shift === 0
              ? "· inside your time zone."
              : `· ${shift === 1 ? "an hour" : `${formatHours(shift)} hours`} from yours${required ? "" : ", but only preferred"}.`}
          </span>
        )}
      </p>
      {hours.evidence && <Evidence>{hours.evidence}</Evidence>}
    </div>
  );
}

function formatHours(hours: number): string {
  return Number.isInteger(hours) ? String(hours) : hours.toFixed(1);
}

/// SourceLocations is what the source itself listed, read out when it is
/// short and behind a disclosure when it is a long list of countries.
function SourceLocations({ source, locations }: { source: string; locations: string[] }) {
  if (locations.length <= 6) {
    return (
      <p className="mt-1 text-xs text-muted-foreground select-text">
        {source} lists: {locations.join(" · ")}
      </p>
    );
  }
  return (
    <details className="mt-1 text-xs text-muted-foreground">
      <summary className="cursor-default select-none hover:text-subtle-foreground">
        {source} lists {locations.length} countries
      </summary>
      <p className="mt-1 leading-relaxed select-text">{locations.join(" · ")}</p>
    </details>
  );
}

/// Evidence is the posting's own sentence behind a label.
function Evidence({ children }: { children: string }) {
  return (
    <blockquote className="mt-1 border-l-2 border-border pl-2.5 text-xs leading-relaxed text-muted-foreground select-text">
      “{children}”
    </blockquote>
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

function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}
