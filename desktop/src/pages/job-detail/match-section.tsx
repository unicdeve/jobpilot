import { CircleAlert, CircleCheck, Sparkles } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { DetailSection } from "../../components/page";
import { Thinking } from "../../components/thinking";
import { Button } from "../../components/ui/button";
import { relativeTime, scoreTone } from "../../lib/format";
import { useAnalyzeMatch, useAnalyzing } from "../../lib/queries/match.queries";
import { useToast } from "../../lib/toast";
import type { JobMatch } from "../../lib/types";

export function MatchSection({
  candidateId,
  jobId,
  match,
}: {
  candidateId: string | null;
  jobId: string;
  match: JobMatch | undefined;
}) {
  const analyzing = useAnalyzing(jobId);
  if (!candidateId) {
    return (
      <DetailSection title="Match">
        <p className="text-sm text-muted-foreground">
          Matching compares this job with your profile.{" "}
          <Link to="/profile" className="text-foreground underline">
            Create a profile
          </Link>{" "}
          to analyze it.
        </p>
      </DetailSection>
    );
  }
  if (!match) {
    return (
      <DetailSection title="Match">
        {analyzing ? (
          <Thinking state="solving">Comparing the posting with your profile…</Thinking>
        ) : (
          <p className="text-sm text-muted-foreground">
            Not analyzed yet. Analyze match compares the posting with your profile and explains the fit.
          </p>
        )}
      </DetailSection>
    );
  }

  return (
    <DetailSection title="Match">
      {analyzing && (
        <Thinking state="solving" className="mb-4">
          Comparing the posting with your profile again. This analysis stays until the new one is ready.
        </Thinking>
      )}
      <div className="flex items-start gap-4">
        <div
          className={`flex size-14 shrink-0 flex-col items-center justify-center rounded-xl ${scoreTone(match.score)}`}
        >
          <span className="text-xl leading-none font-semibold tabular-nums">{match.score}</span>
          <span className="mt-0.5 text-[10px] font-medium opacity-80">of 100</span>
        </div>
        <div className="min-w-0">
          <p className="text-sm leading-relaxed text-subtle-foreground select-text">{match.rationale}</p>
          <p className="mt-1.5 text-xs text-muted-foreground">
            Confidence {Math.round(match.confidence * 100)}% · analyzed {relativeTime(match.created_at) ?? "recently"}
            {match.model_name && ` with ${match.model_name}`}
          </p>
        </div>
      </div>
      <div className="mt-5 grid gap-5 sm:grid-cols-2">
        <FindingList title="Strengths" items={match.strengths} icon={<CircleCheck className="text-success" />} />
        <FindingList title="Gaps" items={match.gaps} icon={<CircleAlert className="text-warning" />} />
      </div>
    </DetailSection>
  );
}

function FindingList({ title, items, icon }: { title: string; items: string[] | undefined; icon: ReactNode }) {
  if (!items?.length) return null;
  return (
    <div>
      <p className="mb-2 text-xs font-semibold text-subtle-foreground">{title}</p>
      <ul className="space-y-2">
        {items.map((item, i) => (
          // biome-ignore lint/suspicious/noArrayIndexKey: an analysis's reasons are shown as written, never reordered, and can repeat
          <li key={i} className="flex gap-2 text-[13px] leading-snug text-subtle-foreground select-text">
            <span className="mt-0.5 shrink-0 [&_svg]:size-3.5">{icon}</span>
            {item}
          </li>
        ))}
      </ul>
    </div>
  );
}

export function AnalyzeButton({
  candidateId,
  jobId,
  analyzed,
}: {
  candidateId: string;
  jobId: string;
  analyzed: boolean;
}) {
  const { notifyError } = useToast();
  const analyzeMatch = useAnalyzeMatch({ onError: notifyError });
  return (
    <Button
      size="sm"
      variant="outline"
      onClick={() => analyzeMatch.mutate({ candidateId, jobId })}
      disabled={analyzeMatch.isPending}
    >
      <Sparkles data-icon="inline-start" />
      {analyzeMatch.isPending ? "Analyzing…" : analyzed ? "Re-analyze" : "Analyze match"}
    </Button>
  );
}
