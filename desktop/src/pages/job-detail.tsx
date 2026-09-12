import { openUrl } from "@tauri-apps/plugin-opener";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { FormField } from "../components/form-field";
import { Markdown } from "../components/markdown";
import { StatusBadge } from "../components/status-badge";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardTitle } from "../components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
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
import type { ApplicationMode, JobMatch } from "../lib/types";

const MODE_LABELS: Record<ApplicationMode, string> = {
  assist: "Assist",
  review: "Review",
  autopilot: "Autopilot",
};

/// JobDetail is the hub the rest of the app navigates through: everything
/// the user can do with a job happens here, rather than by copying its ID to
/// another screen. See docs/00_product/03_user_flows.md — "user opens job →
/// loads candidate context → match score persisted", and "user selects job →
/// system generates cover letter".
export function JobDetail() {
  const { id } = useParams<{ id: string }>();
  const { candidateId } = useCandidateId();
  const { data: job, error, isPending } = useJob(id);
  const { data: matches } = useMatches(candidateId);
  const { data: applications } = useApplications(candidateId);

  const match = matches?.find((m) => m.job_id === id);
  const existing = applications?.filter((a) => a.job_id === id) ?? [];

  if (error) return <p className="text-sm text-red-600">{String(error.message)}</p>;
  if (isPending || !job) return <p className="text-sm text-slate-500">Loading…</p>;

  return (
    <div className="max-w-3xl space-y-4">
      <div>
        <Link to="/jobs" className="text-sm text-slate-500 hover:underline">
          ← Jobs
        </Link>
        <h1 className="mt-1 text-xl font-semibold">{job.title}</h1>
        <p className="text-sm text-slate-600">{job.company_name}</p>
        <p className="mt-1 text-xs text-slate-500">
          {job.remote_type} · {job.employment_type} ·{" "}
          {job.locations?.join(", ") || "location unspecified"}
        </p>
        {job.canonical_url && (
          <a
            href={job.canonical_url}
            className="mt-1 inline-block text-xs text-slate-500 hover:underline"
            onClick={(event) => {
              // Without this the webview navigates to the job board and the
              // app is simply gone; the posting belongs in a real browser.
              event.preventDefault();
              void openUrl(job.canonical_url);
            }}
          >
            {job.canonical_url}
          </a>
        )}
      </div>

      <MatchPanel candidateId={candidateId} jobId={job.id} match={match} />

      <ApplyPanel candidateId={candidateId} jobId={job.id} existingCount={existing.length} />

      {existing.length > 0 && (
        <Card>
          <CardContent>
            <CardTitle>Applications for this job</CardTitle>
            <div className="mt-2 space-y-1">
              {existing.map((app) => (
                <Link
                  key={app.id}
                  to={`/applications/${app.id}`}
                  className="flex items-center justify-between rounded-md px-2 py-1 text-sm hover:bg-slate-100"
                >
                  <span className="text-slate-600">Mode: {app.mode}</span>
                  <StatusBadge value={app.state} />
                </Link>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {job.skills?.length > 0 && (
        <Card>
          <CardContent>
            <CardTitle>Skills</CardTitle>
            <p className="mt-2 text-sm text-slate-700">{job.skills.join(", ")}</p>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardContent>
          <CardTitle>Description</CardTitle>
          {job.description_markdown ? (
            <div className="mt-2">
              <Markdown>{job.description_markdown}</Markdown>
            </div>
          ) : (
            <p className="mt-2 text-sm text-slate-500">No description was captured for this job.</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function MatchPanel({
  candidateId,
  jobId,
  match,
}: {
  candidateId: string | null;
  jobId: string;
  match: JobMatch | undefined;
}) {
  const { notify } = useToast();
  const analyzeMatch = useAnalyzeMatch({ onError: (err) => notify(String(err)) });

  const analyze = () => {
    if (!candidateId) return;
    analyzeMatch.mutate({ candidateId, jobId });
  };

  return (
    <Card>
      <CardContent>
        <div className="flex items-center justify-between">
          <CardTitle>Match</CardTitle>
          <Button
            variant="secondary"
            onClick={analyze}
            disabled={!candidateId || analyzeMatch.isPending}
          >
            {analyzeMatch.isPending ? "Analyzing…" : match ? "Re-analyze" : "Analyze match"}
          </Button>
        </div>

        {!candidateId ? (
          <p className="mt-2 text-sm text-slate-500">
            Create a profile on the <Link to="/profile" className="underline">Profile</Link> page to
            analyze this job.
          </p>
        ) : !match ? (
          <p className="mt-2 text-sm text-slate-500">This job has not been analyzed yet.</p>
        ) : (
          <>
            <p className="mt-2 text-sm font-medium">
              Score {match.score}/100{" "}
              <span className="font-normal text-slate-500">
                (confidence {match.confidence.toFixed(2)})
              </span>
            </p>
            <p className="mt-1 text-sm text-slate-700">{match.rationale}</p>
            <div className="mt-2 grid grid-cols-2 gap-2 text-xs">
              <div>
                <p className="font-medium text-slate-600">Strengths</p>
                <ul className="list-inside list-disc text-slate-600">
                  {match.strengths?.map((s, i) => <li key={i}>{s}</li>)}
                </ul>
              </div>
              <div>
                <p className="font-medium text-slate-600">Gaps</p>
                <ul className="list-inside list-disc text-slate-600">
                  {match.gaps?.map((g, i) => <li key={i}>{g}</li>)}
                </ul>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function ApplyPanel({
  candidateId,
  jobId,
  existingCount,
}: {
  candidateId: string | null;
  jobId: string;
  existingCount: number;
}) {
  const { notify } = useToast();
  const navigate = useNavigate();
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

  const create = () => {
    if (!candidateId) return;
    createApplication.mutate({ jobId, candidateId, resumeId: resumeId ?? "", mode });
  };

  if (!candidateId) return null;

  return (
    <Card>
      <CardContent>
        <CardTitle>
          {existingCount > 0 ? "Start another application" : "Start an application"}
        </CardTitle>
        <div className="mt-2 flex flex-wrap items-end gap-3">
          <div className="min-w-48 flex-1">
            <FormField label="Resume" htmlFor="resume">
              <Select
                items={resumeLabels}
                value={resumeId}
                onValueChange={(value) => setResumeId(value)}
              >
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
          </div>
          <div className="min-w-32">
            <FormField label="Mode" htmlFor="mode">
              <Select
                items={MODE_LABELS}
                value={mode}
                onValueChange={(value) => setMode(value as ApplicationMode)}
              >
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
          <Button onClick={create} disabled={createApplication.isPending}>
            {createApplication.isPending ? "Creating…" : "Create application"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
