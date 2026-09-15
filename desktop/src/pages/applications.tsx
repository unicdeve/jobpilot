import { Link } from "react-router-dom";
import { EmptyState, fillsContentArea } from "../components/empty-state";
import { StatusBadge } from "../components/status-badge";
import { Card, CardContent } from "../components/ui/card";
import { useApplications, useCandidateId, useJobsByID } from "../lib/queries";
import type { Application, ApplicationState } from "../lib/types";

/// Applications are grouped by who the next move belongs to. A flat list
/// sorted by nothing in particular buries the two that need the user today
/// under thirty that are waiting on an employer — and an approved
/// application that was never actually sent looks exactly like one that was.
const GROUPS: { title: string; hint: string; states: ApplicationState[] }[] = [
  {
    title: "Waiting on you",
    hint: "Approve these, or tell JobPilot you sent them.",
    states: ["READY_FOR_REVIEW", "APPROVED"],
  },
  {
    title: "In progress",
    hint: "Being prepared, or waiting on an employer.",
    states: ["DISCOVERED", "MATCHED", "PREPARING", "SUBMITTED", "INTERVIEWING", "OFFERED", "FAILED"],
  },
  {
    title: "Closed",
    hint: "Nothing left to do.",
    states: ["REJECTED", "WITHDRAWN", "NO_RESPONSE"],
  },
];

function groupOf(application: Application) {
  return GROUPS.findIndex((group) => group.states.includes(application.state));
}

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
        <div className="space-y-6">
          {GROUPS.map((group, index) => {
            const inGroup = applications.filter((app) => groupOf(app) === index);
            if (inGroup.length === 0) return null;

            return (
              <section key={group.title}>
                <div className="mb-2 flex items-baseline gap-2">
                  <h2 className="text-sm font-semibold text-slate-900">{group.title}</h2>
                  <span className="text-xs text-slate-500">{group.hint}</span>
                </div>
                <div className="space-y-2">
                  {inGroup.map((app) => {
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
                                    <span className="font-normal text-slate-500">
                                      at {job.company_name}
                                    </span>
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
              </section>
            );
          })}
        </div>
      )}
    </div>
  );
}
