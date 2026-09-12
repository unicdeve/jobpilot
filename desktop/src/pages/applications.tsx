import { Link } from "react-router-dom";
import { StatusBadge } from "../components/status-badge";
import { Card, CardContent } from "../components/ui/card";
import { useApplications, useCandidateId, useJobsByID } from "../lib/queries";

export function Applications() {
  const { candidateId } = useCandidateId();
  const { data: applications, error, isPending } = useApplications(candidateId);

  // An application records only its job_id; the jobs give each row a title.
  const jobs = useJobsByID(applications?.map((a) => a.job_id) ?? []);

  if (!candidateId) {
    return (
      <p className="text-sm text-slate-500">
        No candidate profile found — create one on the{" "}
        <Link to="/profile" className="underline">
          Profile
        </Link>{" "}
        page first.
      </p>
    );
  }

  if (error) return <p className="text-sm text-red-600">{String(error.message)}</p>;
  if (isPending) return <p className="text-sm text-slate-500">Loading…</p>;

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Applications</h1>

      {applications.length === 0 ? (
        <p className="text-sm text-slate-500">
          No applications yet — open a job from the{" "}
          <Link to="/jobs" className="underline">
            Jobs
          </Link>{" "}
          page and start one there.
        </p>
      ) : (
        <div className="space-y-2">
          {applications.map((app) => {
            const job = jobs.get(app.job_id);
            return (
              <Link key={app.id} to={`/applications/${app.id}`} className="block">
                <Card className="hover:ring-foreground/20">
                  <CardContent className="flex items-center justify-between gap-3">
                    <div>
                      <p className="font-medium">
                        {job ? (
                          <>
                            {job.title}{" "}
                            <span className="font-normal text-slate-500">at {job.company_name}</span>
                          </>
                        ) : (
                          <span className="text-slate-500">Loading job…</span>
                        )}
                      </p>
                      <p className="text-xs text-slate-500">Mode: {app.mode}</p>
                    </div>
                    <StatusBadge value={app.state} />
                  </CardContent>
                </Card>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
