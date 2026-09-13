import { Link } from "react-router-dom";
import { EmptyState, fillsContentArea } from "../components/empty-state";
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
      <EmptyState
        title="No candidate profile yet"
        description="Applications are generated from your profile, so JobPilot needs one before you can start any."
        actionLabel="Create your profile"
        actionTo="/profile"
      />
    );
  }

  if (error) return <p className="text-sm text-red-600">{String(error.message)}</p>;
  if (isPending) return <p className="text-sm text-slate-500">Loading…</p>;

  return (
    // A full-height column so an empty state below the heading centres in
    // what is left, rather than pushing the page past the fold.
    <div className={`flex flex-col ${fillsContentArea}`}>
      <h1 className="mb-4 text-xl font-semibold">Applications</h1>

      {applications.length === 0 ? (
        <EmptyState
          className="min-h-0 flex-1"
          title="No applications yet"
          description="Open a job and start an application there — its cover letter and questions then live on this page."
          actionLabel="Browse jobs"
          actionTo="/jobs"
        />
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
