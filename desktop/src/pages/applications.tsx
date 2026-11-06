import { Send } from "lucide-react";
import { Link, Outlet, useMatch } from "react-router-dom";
import { ListItem, ListNote, ListPane, NoSelection } from "../components/list-pane";
import { SplitView } from "../components/split-view";
import { StatusBadge } from "../components/status-badge";
import { relativeTime } from "../lib/format";
import { useApplications } from "../lib/queries/application.queries";
import { useJobsByID } from "../lib/queries/job.queries";
import { useCandidateId } from "../lib/queries/profile.queries";
import type { Application, ApplicationState, Job } from "../lib/types";

/// Applications are grouped by who the next move belongs to. A flat list
/// sorted by nothing in particular buries the ones the user still has to
/// send under thirty that are waiting on an employer, and a ready-to-send
/// application that was never actually sent looks exactly like one that was.
const GROUPS: { title: string; hint: string; states: ApplicationState[] }[] = [
  {
    title: "Waiting on you",
    hint: "Apply, then tell JobPilot you sent them.",
    states: ["APPROVED", "MATCHED", "DISCOVERED"],
  },
  {
    title: "In progress",
    hint: "Sent, and waiting on an employer.",
    states: ["SUBMITTED", "INTERVIEWING", "OFFERED"],
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
  const hasSelection = useMatch("/applications/:id") !== null;
  return (
    <SplitView
      hasSelection={hasSelection}
      list={
        <ListPane title="Applications">
          <ApplicationList />
        </ListPane>
      }
      detail={<Outlet />}
    />
  );
}

export function NoApplicationSelected() {
  return (
    <NoSelection
      icon={<Send />}
      title="No application selected"
      description="Choose an application to review its answers, cover letter and next step."
    />
  );
}

function ApplicationList() {
  const { candidateId } = useCandidateId();
  const { data: applications, error, isPending } = useApplications(candidateId);

  // An application records only its job_id; the jobs give each row a title.
  const jobs = useJobsByID(applications?.map((a) => a.job_id) ?? []);

  if (!candidateId) {
    return (
      <ListNote>
        Applications are built from your profile.{" "}
        <Link to="/profile" className="underline">
          Create one
        </Link>{" "}
        to start.
      </ListNote>
    );
  }
  if (error) return <ListNote tone="error">{error.message}</ListNote>;
  if (isPending) return <ListNote>Loading…</ListNote>;
  if (applications.length === 0) {
    return (
      <ListNote>
        No applications yet. Open a job in{" "}
        <Link to="/jobs" className="underline">
          Jobs
        </Link>{" "}
        and choose Start application.
      </ListNote>
    );
  }

  return (
    <>
      {GROUPS.map((group, index) => {
        const inGroup = applications
          .filter((app) => groupOf(app) === index)
          .sort((a, b) => b.updated_at.localeCompare(a.updated_at));
        if (inGroup.length === 0) return null;

        return (
          <section key={group.title}>
            <div className="sticky top-0 z-10 border-b border-border bg-background/90 px-4 pt-3 pb-2 backdrop-blur-sm">
              <h2 className="text-xs font-semibold">
                {group.title} <span className="font-normal text-muted-foreground">{inGroup.length}</span>
              </h2>
              <p className="text-xs text-muted-foreground">{group.hint}</p>
            </div>
            {inGroup.map((app) => (
              <ApplicationRow key={app.id} application={app} job={jobs.get(app.job_id)} />
            ))}
          </section>
        );
      })}
    </>
  );
}

function ApplicationRow({ application, job }: { application: Application; job: Job | undefined }) {
  return (
    <ListItem to={`/applications/${application.id}`}>
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="line-clamp-2 text-[13px] leading-snug font-medium">
            {job ? job.title : <span className="text-muted-foreground">Loading job…</span>}
          </p>
          <p className="mt-0.5 truncate text-xs text-subtle-foreground">{job?.company_name}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Updated {relativeTime(application.updated_at) ?? "recently"}
          </p>
        </div>
        <StatusBadge value={application.state} className="mt-0.5 shrink-0" />
      </div>
    </ListItem>
  );
}
