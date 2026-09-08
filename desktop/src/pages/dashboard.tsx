import { useEffect, useState } from "react";
import { Card, CardContent, CardTitle } from "../components/ui/card";
import { useApi } from "../lib/daemon";
import type { Application, ApplicationState } from "../lib/types";

export function Dashboard() {
  const api = useApi();
  const [counts, setCounts] = useState<{
    jobs: number;
    matches: number;
    applications: number;
    byState: Partial<Record<ApplicationState, number>>;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        const profile = await api.getProfile();
        const [jobs, matches, applications] = await Promise.all([
          api.searchJobs({}),
          api.listMatches(profile.id),
          api.listApplications(profile.id),
        ]);
        if (cancelled) return;

        const byState: Partial<Record<ApplicationState, number>> = {};
        for (const app of applications as Application[]) {
          byState[app.state] = (byState[app.state] ?? 0) + 1;
        }
        setCounts({ jobs: jobs.length, matches: matches.length, applications: applications.length, byState });
      } catch (err) {
        if (!cancelled) setError(String(err));
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [api]);

  if (error) {
    return <p className="text-sm text-red-600">{error} — has a candidate profile been created yet?</p>;
  }

  if (!counts) {
    return <p className="text-sm text-slate-500">Loading…</p>;
  }

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Dashboard</h1>
      <div className="grid grid-cols-3 gap-4">
        <Card>
          <CardContent>
            <CardTitle>Jobs discovered</CardTitle>
            <p className="mt-2 text-3xl font-semibold">{counts.jobs}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <CardTitle>Matches analyzed</CardTitle>
            <p className="mt-2 text-3xl font-semibold">{counts.matches}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <CardTitle>Applications</CardTitle>
            <p className="mt-2 text-3xl font-semibold">{counts.applications}</p>
          </CardContent>
        </Card>
      </div>
      {Object.keys(counts.byState).length > 0 && (
        <Card className="mt-4">
          <CardContent>
            <CardTitle>Applications by state</CardTitle>
            <div className="mt-2 flex flex-wrap gap-3 text-sm">
              {Object.entries(counts.byState).map(([state, count]) => (
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
