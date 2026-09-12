import { Card, CardContent, CardTitle } from "../components/ui/card";
import { useApplications, useCandidateId, useJobCount, useMatches } from "../lib/queries";
import type { ApplicationState } from "../lib/types";

export function Dashboard() {
  const { candidateId, isPending: profilePending, isError: noProfile } = useCandidateId();

  // Reads stored history; deliberately not a live source search, which would
  // put a network round trip and a database write in the path of simply
  // opening the app.
  const jobs = useJobCount();
  const matches = useMatches(candidateId);
  const applications = useApplications(candidateId);

  if (noProfile) {
    return (
      <p className="text-sm text-slate-500">
        No candidate profile yet — create one on the Profile page to get started.
      </p>
    );
  }

  const error = jobs.error ?? matches.error ?? applications.error;
  if (error) {
    return <p className="text-sm text-red-600">{String(error.message)}</p>;
  }

  // The match and application queries stay idle until the profile resolves
  // a candidate ID, so waiting on their data covers the profile load too.
  if (profilePending || jobs.data === undefined || !matches.data || !applications.data) {
    return <p className="text-sm text-slate-500">Loading…</p>;
  }

  const byState: Partial<Record<ApplicationState, number>> = {};
  for (const app of applications.data) {
    byState[app.state] = (byState[app.state] ?? 0) + 1;
  }

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Dashboard</h1>
      <div className="grid grid-cols-3 gap-4">
        <Card>
          <CardContent>
            <CardTitle>Jobs discovered</CardTitle>
            <p className="mt-2 text-3xl font-semibold">{jobs.data}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <CardTitle>Matches analyzed</CardTitle>
            <p className="mt-2 text-3xl font-semibold">{matches.data.length}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <CardTitle>Applications</CardTitle>
            <p className="mt-2 text-3xl font-semibold">{applications.data.length}</p>
          </CardContent>
        </Card>
      </div>
      {Object.keys(byState).length > 0 && (
        <Card className="mt-4">
          <CardContent>
            <CardTitle>Applications by state</CardTitle>
            <div className="mt-2 flex flex-wrap gap-3 text-sm">
              {Object.entries(byState).map(([state, count]) => (
                <span key={state}>
                  {state}: <strong>{count}</strong>
                </span>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
