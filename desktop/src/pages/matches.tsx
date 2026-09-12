import { useState } from "react";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { useToast } from "../lib/toast";
import { useAnalyzeMatch, useCandidateId, useMatches } from "../lib/queries";

export function Matches() {
  const { notify } = useToast();
  const { candidateId } = useCandidateId();
  const { data: matches } = useMatches(candidateId);
  const [jobId, setJobId] = useState("");

  const analyzeMatch = useAnalyzeMatch({
    onSuccess: () => setJobId(""),
    onError: (err) => notify(String(err)),
  });

  const analyze = () => {
    if (!candidateId || !jobId.trim()) return;
    analyzeMatch.mutate({ candidateId, jobId: jobId.trim() });
  };

  if (!candidateId) {
    return <p className="text-sm text-slate-500">No candidate profile found — create one on the Profile page first.</p>;
  }

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Matches</h1>

      <Card className="mb-4">
        <CardContent className="flex items-end gap-3">
          <div className="flex-1">
            <Input
              placeholder="Job ID to analyze (see Jobs page)…"
              value={jobId}
              onChange={(e) => setJobId(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && analyze()}
            />
          </div>
          <Button onClick={analyze} disabled={analyzeMatch.isPending}>
            {analyzeMatch.isPending ? "Analyzing…" : "Analyze"}
          </Button>
        </CardContent>
      </Card>

      {!matches || matches.length === 0 ? (
        <p className="text-sm text-slate-500">No matches analyzed yet.</p>
      ) : (
        <div className="space-y-3">
          {matches.map((m) => (
            <Card key={m.id}>
              <CardContent>
                <CardTitle>
                  Score {m.score}/100 <span className="font-normal text-slate-500">(confidence {m.confidence.toFixed(2)})</span>
                </CardTitle>
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
                <p className="mt-2 text-xs text-slate-400">Job: {m.job_id}</p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
