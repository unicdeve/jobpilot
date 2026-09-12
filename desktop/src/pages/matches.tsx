import { Link } from "react-router-dom";
import { Card, CardContent, CardTitle } from "../components/ui/card";
import { useCandidateId, useJobsByID, useMatches } from "../lib/queries";

export function Matches() {
  const { candidateId } = useCandidateId();
  const { data: matches, error, isPending } = useMatches(candidateId);

  // A match records only its job_id, so the jobs behind them are fetched to
  // give each one a title to show and a link to follow.
  const jobs = useJobsByID(matches?.map((m) => m.job_id) ?? []);

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
      <h1 className="mb-4 text-xl font-semibold">Matches</h1>

      {matches.length === 0 ? (
        <p className="text-sm text-slate-500">
          No matches analyzed yet — open a job from the{" "}
          <Link to="/jobs" className="underline">
            Jobs
          </Link>{" "}
          page and analyze it there.
        </p>
      ) : (
        <div className="space-y-3">
          {matches.map((m) => {
            const job = jobs.get(m.job_id);
            return (
              <Link key={m.id} to={`/jobs/${m.job_id}`} className="block">
                <Card className="hover:ring-foreground/20">
                  <CardContent>
                    <div className="flex items-baseline justify-between gap-3">
                      <CardTitle>
                        {job ? (
                          <>
                            {job.title}{" "}
                            <span className="font-normal text-slate-500">at {job.company_name}</span>
                          </>
                        ) : (
                          <span className="text-slate-500">Loading job…</span>
                        )}
                      </CardTitle>
                      <span className="shrink-0 text-sm font-medium">
                        {m.score}/100{" "}
                        <span className="font-normal text-slate-500">
                          (confidence {m.confidence.toFixed(2)})
                        </span>
                      </span>
                    </div>
                    <p className="mt-1 text-sm text-slate-700">{m.rationale}</p>
                    <div className="mt-2 grid grid-cols-2 gap-2 text-xs">
                      <div>
                        <p className="font-medium text-slate-600">Strengths</p>
                        <ul className="list-inside list-disc text-slate-600">
                          {m.strengths?.map((s, i) => <li key={i}>{s}</li>)}
                        </ul>
                      </div>
                      <div>
                        <p className="font-medium text-slate-600">Gaps</p>
                        <ul className="list-inside list-disc text-slate-600">
                          {m.gaps?.map((g, i) => <li key={i}>{g}</li>)}
                        </ul>
                      </div>
                    </div>
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
