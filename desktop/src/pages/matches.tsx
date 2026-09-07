import { useEffect, useState } from "react";
import { Button } from "../components/ui/button";
import { Card, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/field";
import { useApi } from "../lib/daemon";
import { useToast } from "../lib/toast";
import type { JobMatch } from "../lib/types";

export function Matches() {
  const api = useApi();
  const { notify } = useToast();
  const [candidateId, setCandidateId] = useState<string | null>(null);
  const [matches, setMatches] = useState<JobMatch[]>([]);
  const [jobId, setJobId] = useState("");
  const [analyzing, setAnalyzing] = useState(false);

  const refresh = async (cid: string) => setMatches(await api.listMatches(cid));

  useEffect(() => {
    api
      .getProfile()
      .then(async (profile) => {
        setCandidateId(profile.id);
        await refresh(profile.id);
      })
      .catch((err) => notify(String(err)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api]);

  const analyze = async () => {
    if (!candidateId || !jobId.trim()) return;
    setAnalyzing(true);
    try {
      await api.analyzeMatch(candidateId, jobId.trim());
      await refresh(candidateId);
      setJobId("");
    } catch (err) {
      notify(String(err));
    } finally {
      setAnalyzing(false);
    }
  };

  if (!candidateId) {
    return <p className="text-sm text-slate-500">No candidate profile found — create one on the Profile page first.</p>;
  }

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Matches</h1>

      <Card className="mb-4 flex items-end gap-3">
        <div className="flex-1">
          <Input
            placeholder="Job ID to analyze (see Jobs page)…"
            value={jobId}
            onChange={(e) => setJobId(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && analyze()}
          />
        </div>
        <Button onClick={analyze} disabled={analyzing}>
          {analyzing ? "Analyzing…" : "Analyze"}
        </Button>
      </Card>

      {matches.length === 0 ? (
        <p className="text-sm text-slate-500">No matches analyzed yet.</p>
      ) : (
        <div className="space-y-3">
          {matches.map((m) => (
            <Card key={m.id}>
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
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
