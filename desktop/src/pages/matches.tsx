import { Link } from "react-router-dom";
import { EmptyState, fillsContentArea } from "../components/empty-state";
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
      <EmptyState
        title="No candidate profile yet"
        description="Matching compares a job against your profile, so JobPilot needs one before it can analyze anything."
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
      <h1 className="mb-4 text-xl font-semibold">Matches</h1>

      {matches.length === 0 ? (
        <EmptyState
          className="min-h-0 flex-1"
          title="No matches analyzed yet"
          description="Open a job and analyze it there — the score, strengths and gaps appear on this page afterwards."
          actionLabel="Browse jobs"
          actionTo="/jobs"
        />
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
