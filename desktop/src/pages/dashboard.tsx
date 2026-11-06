import { ChevronRight, Radar, TriangleAlert, X } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { EmptyState } from "../components/empty-state";
import { Page, Panel, Section } from "../components/page";
import { StatusBadge } from "../components/status-badge";
import { ErrorLine, Loading } from "../components/status-line";
import { runKind, runSubject } from "../lib/activity";
import { dateTime, relativeTime, scoreTone } from "../lib/format";
import { useInfiniteActivity } from "../lib/queries/activity.queries";
import { useApplications } from "../lib/queries/application.queries";
import { useJobCount, useJobsByID } from "../lib/queries/job.queries";
import { useMatches } from "../lib/queries/match.queries";
import { useMarkNotificationsSeen, useUnseenNotifications } from "../lib/queries/notification.queries";
import { useCandidateId, useProfile } from "../lib/queries/profile.queries";
import type { Application, ApplicationState, Notification } from "../lib/types";

const WAITING: ApplicationState[] = ["APPROVED", "MATCHED", "DISCOVERED"];
const CLOSED: ApplicationState[] = ["REJECTED", "WITHDRAWN", "NO_RESPONSE"];

/// Dashboard is where the day starts: what needs the user, what is worth
/// applying to, and what ran on their behalf. Every figure on it is a link
/// to the list it counts.
export function Dashboard() {
  const { candidateId, isPending: profilePending, noProfile } = useCandidateId();
  const { data: profile } = useProfile();

  // Reads stored history; deliberately not a live source search, which would
  // put a network round trip and a database write in the path of simply
  // opening the app.
  const jobs = useJobCount();
  const matches = useMatches(candidateId);
  const applications = useApplications(candidateId);

  if (noProfile) {
    // Nothing on this screen can be filled in until a candidate exists, so
    // the whole screen becomes the invitation to create one.
    return (
      <EmptyState
        title="Welcome to JobPilot"
        description="Start by creating your candidate profile. JobPilot uses it to match you against jobs, and keeps your details ready to copy when you apply."
        actionLabel="Create your profile"
        actionTo="/profile"
      />
    );
  }

  const error = jobs.error ?? matches.error ?? applications.error;
  if (error) return <ErrorLine error={error} />;

  // The match and application queries stay idle until the profile resolves
  // a candidate ID, so waiting on their data covers the profile load too.
  if (profilePending || jobs.data === undefined || !matches.data || !applications.data) return <Loading />;

  const apps = applications.data;
  // Ready to send leads: it is the state an application gets lost in.
  const waiting = apps
    .filter((a) => WAITING.includes(a.state))
    .sort((a, b) => WAITING.indexOf(a.state) - WAITING.indexOf(b.state));
  const open = apps.filter((a) => !CLOSED.includes(a.state));
  const appliedJobs = new Set(apps.map((a) => a.job_id));

  return (
    <Page title={profile?.first_name ? `Welcome back, ${profile.first_name}` : "Dashboard"} width="wide">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat to="/jobs" label="Jobs found" value={jobs.data} />
        <Stat to="/jobs?view=analyzed" label="Analyzed" value={matches.data.length} />
        <Stat to="/applications" label="Open applications" value={open.length} />
        <Stat to="/applications" label="Waiting on you" value={waiting.length} emphasis={waiting.length > 0} />
      </div>

      <div className="mt-8 grid gap-8 lg:grid-cols-2">
        <div>
          <NeedsYou waiting={waiting} />
          <BestMatches matches={matches.data.filter((m) => !appliedJobs.has(m.job_id))} />
        </div>
        <div>
          <WhileYouWereAway />
          <RecentActivity />
        </div>
      </div>
    </Page>
  );
}

function Stat({ to, label, value, emphasis }: { to: string; label: string; value: number; emphasis?: boolean }) {
  return (
    <Link
      to={to}
      className="group rounded-lg border border-border bg-card px-4 py-3 outline-none hover:bg-accent/50 focus-visible:ring-2 focus-visible:ring-ring"
    >
      <p className="flex items-center justify-between text-xs font-medium text-muted-foreground">
        {label}
        <ChevronRight className="size-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
      </p>
      <p className={`mt-1 text-2xl font-semibold tabular-nums ${emphasis ? "text-primary" : ""}`}>{value}</p>
    </Link>
  );
}

function RowLink({ to, children, trailing }: { to: string; children: ReactNode; trailing?: ReactNode }) {
  return (
    <Link to={to} className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/50">
      <div className="min-w-0 flex-1">{children}</div>
      {trailing}
      <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
    </Link>
  );
}

/// NeedsYou is the part of the dashboard that asks for something: an
/// application the candidate has not sent yet. Ready-to-send is the state
/// an application gets lost in, so it leads.
function NeedsYou({ waiting }: { waiting: Application[] }) {
  const items = waiting;
  const jobs = useJobsByID(items.map((a) => a.job_id));

  return (
    <Section title="Needs you">
      {items.length === 0 ? (
        <Panel className="px-4 py-3">
          <p className="text-sm text-muted-foreground">Nothing is waiting on you.</p>
        </Panel>
      ) : (
        <Panel className="divide-y divide-border">
          {items.slice(0, 6).map((app) => {
            const job = jobs.get(app.job_id);
            return (
              <RowLink key={app.id} to={`/applications/${app.id}`} trailing={<StatusBadge value={app.state} />}>
                <p className="truncate text-[13px] font-medium">{job?.title ?? "Loading…"}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {job?.company_name}
                  {app.state === "APPROVED" && " · ready to send. Did you send it?"}
                </p>
              </RowLink>
            );
          })}
          {items.length > 6 && (
            <Link to="/applications" className="block px-4 py-2 text-xs text-muted-foreground hover:bg-accent/50">
              {items.length - 6} more in Applications
            </Link>
          )}
        </Panel>
      )}
    </Section>
  );
}

/// WhileYouWereAway is what JobPilot told the user that they have not seen
/// here yet: what a scheduled scout found, or that it could not run. It is
/// the same list the operating system was given, kept so one it never
/// showed still reaches them (ADR-0028 §4), and it is absent when there is
/// nothing to say.
function WhileYouWereAway() {
  const { data: notifications } = useUnseenNotifications();
  const markSeen = useMarkNotificationsSeen();
  if (!notifications || notifications.length === 0) return null;

  return (
    <Section
      title="While you were away"
      actions={
        <button
          type="button"
          onClick={() => markSeen.mutate(undefined)}
          className="text-xs text-muted-foreground hover:text-foreground"
        >
          Dismiss all
        </button>
      }
    >
      <Panel className="divide-y divide-border">
        {notifications.slice(0, 5).map((notification) => (
          <NotificationRow
            key={notification.id}
            notification={notification}
            onSeen={() => markSeen.mutate(notification.id)}
          />
        ))}
        {notifications.length > 5 && (
          <p className="px-4 py-2 text-xs text-muted-foreground">{notifications.length - 5} more earlier</p>
        )}
      </Panel>
    </Section>
  );
}

function NotificationRow({ notification, onSeen }: { notification: Notification; onSeen: () => void }) {
  const failed = notification.kind === "scout_failed";
  const Icon = failed ? TriangleAlert : Radar;
  // The run it is about, when there is one; the scouts otherwise.
  const to = notification.run_id ? `/activity/${notification.run_id}` : "/scouts";

  return (
    <div className="flex items-start gap-3 py-2.5 pr-2 pl-4 hover:bg-accent/50">
      <Icon className={`mt-0.5 size-4 shrink-0 ${failed ? "text-warning" : "text-primary"}`} />
      <Link to={to} onClick={onSeen} className="min-w-0 flex-1 outline-none focus-visible:underline">
        <p className="truncate text-[13px] font-medium">{notification.title}</p>
        <p className="text-xs text-muted-foreground">
          {notification.body}{" "}
          <span title={dateTime(notification.created_at)}>· {relativeTime(notification.created_at)}</span>
        </p>
      </Link>
      <button
        type="button"
        onClick={onSeen}
        aria-label={`Dismiss “${notification.title}”`}
        className="rounded p-1 text-muted-foreground outline-none hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        <X className="size-3.5" />
      </button>
    </div>
  );
}

/// BestMatches is the analysed jobs the user has not applied to yet, best
/// first: the list that answers "what should I look at next".
function BestMatches({ matches }: { matches: { id: string; job_id: string; score: number }[] }) {
  const top = [...matches].sort((a, b) => b.score - a.score).slice(0, 5);
  const jobs = useJobsByID(top.map((m) => m.job_id));

  return (
    <Section
      title="Worth a look"
      description="Your best matches that you haven’t applied to."
      actions={
        <Link to="/jobs?view=analyzed" className="text-xs text-muted-foreground hover:text-foreground">
          All analyzed
        </Link>
      }
    >
      {top.length === 0 ? (
        <Panel className="px-4 py-3">
          <p className="text-sm text-muted-foreground">
            Analyze a job from{" "}
            <Link to="/jobs" className="text-foreground underline">
              Jobs
            </Link>{" "}
            to see how well it fits.
          </p>
        </Panel>
      ) : (
        <Panel className="divide-y divide-border">
          {top.map((m) => {
            const job = jobs.get(m.job_id);
            return (
              <RowLink
                key={m.id}
                to={`/jobs/${m.job_id}`}
                trailing={
                  <span className={`rounded-md px-1.5 py-0.5 text-xs font-semibold tabular-nums ${scoreTone(m.score)}`}>
                    {m.score}
                  </span>
                }
              >
                <p className="truncate text-[13px] font-medium">{job?.title ?? "Loading…"}</p>
                <p className="truncate text-xs text-muted-foreground">{job?.company_name}</p>
              </RowLink>
            );
          })}
        </Panel>
      )}
    </Section>
  );
}

function RecentActivity() {
  const { data, error } = useInfiniteActivity({ task_type: "", status: "" }, 8);
  const runs = data?.pages[0]?.runs ?? [];

  return (
    <Section
      title="Recent activity"
      actions={
        <Link to="/activity" className="text-xs text-muted-foreground hover:text-foreground">
          All activity
        </Link>
      }
    >
      {error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : runs.length === 0 ? (
        <Panel className="px-4 py-3">
          <p className="text-sm text-muted-foreground">
            Nothing has run yet. Runs appear here when a scout looks for jobs.
          </p>
        </Panel>
      ) : (
        <Panel className="divide-y divide-border">
          {runs.map((run) => (
            <RowLink key={run.id} to={`/activity/${run.id}`} trailing={<StatusBadge value={run.status} />}>
              <p className="truncate text-[13px] font-medium">{runSubject(run)}</p>
              <p className="truncate text-xs text-muted-foreground">
                {runKind(run)} · {relativeTime(run.started_at)}
              </p>
            </RowLink>
          ))}
        </Panel>
      )}
    </Section>
  );
}
