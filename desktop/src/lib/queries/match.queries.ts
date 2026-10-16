import { skipToken, useMutation, useMutationState, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { JobMatch } from "../types";
import { keys, mutationKeys } from "./keys";
import type { MutationOpts } from "./mutation";

export function useMatches(candidateId: string | null) {
  const api = useApi();
  return useQuery<JobMatch[]>({
    queryKey: keys.matches.list(candidateId ?? ""),
    queryFn: candidateId !== null ? () => api.listMatches(candidateId) : skipToken,
  });
}

/// useJobMatch loads one job's latest match, or null. The job's page reads
/// it this way rather than from useMatches, which leaves out the jobs the
/// user ruled out — a page they can still open.
export function useJobMatch(jobId: string | undefined) {
  const api = useApi();
  return useQuery<JobMatch | null>({
    queryKey: keys.matches.job(jobId ?? ""),
    queryFn: jobId ? () => api.getJobMatch(jobId) : skipToken,
  });
}

interface AnalyzeMatchVars {
  candidateId: string;
  jobId: string;
}

export function useAnalyzeMatch(opts?: MutationOpts<JobMatch, AnalyzeMatchVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: mutationKeys.analysis,
    mutationFn: ({ candidateId, jobId }: AnalyzeMatchVars) => api.analyzeMatch(candidateId, jobId),
    ...opts,
    onSuccess: (match, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.matches.list(variables.candidateId) });
      queryClient.setQueryData(keys.matches.job(variables.jobId), match);
      opts?.onSuccess?.(match, variables, ...rest);
    },
  });
}

/// useAnalyzing says whether a match for this job is being analysed,
/// wherever it was asked for.
export function useAnalyzing(jobId: string): boolean {
  const pending = useMutationState({
    filters: {
      mutationKey: mutationKeys.analysis,
      status: "pending",
      predicate: (mutation) => (mutation.state.variables as AnalyzeMatchVars | undefined)?.jobId === jobId,
    },
  });
  return pending.length > 0;
}
