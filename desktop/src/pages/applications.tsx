import { useState } from "react";
import { Link } from "react-router-dom";
import { FormField } from "../components/form-field";
import { StatusBadge } from "../components/status-badge";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Input } from "../components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import { useApplications, useCandidateId, useCreateApplication } from "../lib/queries";
import { useToast } from "../lib/toast";
import type { ApplicationMode } from "../lib/types";

export function Applications() {
  const { notify } = useToast();
  const { candidateId } = useCandidateId();
  const { data: applications } = useApplications(candidateId);
  const [jobId, setJobId] = useState("");
  const [resumeId, setResumeId] = useState("");
  const [mode, setMode] = useState<ApplicationMode>("review");

  const createApplication = useCreateApplication({
    onSuccess: () => {
      setJobId("");
      setResumeId("");
      notify("Application created.", "success");
    },
    onError: (err) => notify(String(err)),
  });

  const create = () => {
    if (!candidateId || !jobId.trim()) return;
    createApplication.mutate({
      jobId: jobId.trim(),
      candidateId,
      resumeId: resumeId.trim(),
      mode,
    });
  };

  if (!candidateId) {
    return <p className="text-sm text-slate-500">No candidate profile found — create one on the Profile page first.</p>;
  }

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Applications</h1>

      <Card className="mb-4">
        <CardContent className="flex flex-wrap items-end gap-3">
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
              <Select value={mode} onValueChange={(value) => setMode(value as ApplicationMode)}>
                <SelectTrigger id="mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="assist">Assist</SelectItem>
                  <SelectItem value="review">Review</SelectItem>
                  <SelectItem value="autopilot">Autopilot</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
          </div>
          <Button onClick={create} disabled={createApplication.isPending}>
            {createApplication.isPending ? "Creating…" : "Create"}
          </Button>
        </CardContent>
      </Card>

      {!applications || applications.length === 0 ? (
        <p className="text-sm text-slate-500">No applications yet.</p>
      ) : (
        <div className="space-y-2">
          {applications.map((app) => (
            <Link key={app.id} to={`/applications/${app.id}`}>
              <Card className="hover:ring-foreground/20">
                <CardContent className="flex items-center justify-between">
                  <div>
                    <p className="font-medium">Job {app.job_id}</p>
                    <p className="text-xs text-slate-500">Mode: {app.mode}</p>
                  </div>
                  <StatusBadge value={app.state} />
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
