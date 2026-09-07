import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Badge } from "../components/ui/badge";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import { FormField, Input } from "../components/ui/field";
import { useApi } from "../lib/daemon";
import { useToast } from "../lib/toast";
import type { Application, ApplicationMode } from "../lib/types";

export function Applications() {
  const api = useApi();
  const { notify } = useToast();
  const [candidateId, setCandidateId] = useState<string | null>(null);
  const [applications, setApplications] = useState<Application[]>([]);
  const [jobId, setJobId] = useState("");
  const [resumeId, setResumeId] = useState("");
  const [mode, setMode] = useState<ApplicationMode>("review");
  const [creating, setCreating] = useState(false);

  const refresh = async (cid: string) => setApplications(await api.listApplications(cid));

  useEffect(() => {
    api
      .getProfile()
      .then(async (profile) => {
        setCandidateId(profile.id);
        await refresh(profile.id);
      })
      .catch((err) => notify(String(err)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api]);

  const create = async () => {
    if (!candidateId || !jobId.trim()) return;
    setCreating(true);
    try {
      await api.createApplication(jobId.trim(), candidateId, resumeId.trim(), mode);
      await refresh(candidateId);
      setJobId("");
      setResumeId("");
      notify("Application created.", "success");
    } catch (err) {
      notify(String(err));
    } finally {
      setCreating(false);
    }
  };

  if (!candidateId) {
    return <p className="text-sm text-slate-500">No candidate profile found — create one on the Profile page first.</p>;
  }

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Applications</h1>

      <Card className="mb-4 flex flex-wrap items-end gap-3">
        <div className="min-w-40 flex-1">
          <FormField label="Job ID" htmlFor="job-id">
            <Input id="job-id" value={jobId} onChange={(e) => setJobId(e.target.value)} />
          </FormField>
        </div>
        <div className="min-w-40 flex-1">
          <FormField label="Resume ID (optional)" htmlFor="resume-id">
            <Input id="resume-id" value={resumeId} onChange={(e) => setResumeId(e.target.value)} />
          </FormField>
        </div>
        <div className="min-w-32">
          <FormField label="Mode" htmlFor="mode">
            <select
              id="mode"
              className="w-full rounded-md border border-slate-300 px-2.5 py-1.5 text-sm"
              value={mode}
              onChange={(e) => setMode(e.target.value as ApplicationMode)}
            >
              <option value="assist">Assist</option>
              <option value="review">Review</option>
              <option value="autopilot">Autopilot</option>
            </select>
          </FormField>
        </div>
        <Button onClick={create} disabled={creating}>
          {creating ? "Creating…" : "Create"}
        </Button>
      </Card>

      {applications.length === 0 ? (
        <p className="text-sm text-slate-500">No applications yet.</p>
      ) : (
        <div className="space-y-2">
          {applications.map((app) => (
            <Link key={app.id} to={`/applications/${app.id}`}>
              <Card className="flex items-center justify-between hover:border-slate-400">
                <div>
                  <p className="font-medium">Job {app.job_id}</p>
                  <p className="text-xs text-slate-500">Mode: {app.mode}</p>
                </div>
                <Badge value={app.state} />
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
