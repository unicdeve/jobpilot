import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { JobCompany, StarterCompany, WatchedCompany } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useCompanies lists the watched companies, with how each was last checked.
/// A search checks them, so the list is refetched whenever a search ends.
export function useCompanies() {
  const api = useApi();
  return useQuery<WatchedCompany[]>({ queryKey: keys.companies.list, queryFn: () => api.listCompanies() });
}

/// useCompanySuggestions is the starter list, less what is watched.
export function useCompanySuggestions() {
  const api = useApi();
  return useQuery<StarterCompany[]>({
    queryKey: keys.companies.suggestions,
    queryFn: () => api.companySuggestions(),
  });
}

/// useJobCompany is the hiring page a job's posting is on, and the watched
/// company it is, so the job can offer to watch it (spec AC-002).
export function useJobCompany(jobId: string) {
  const api = useApi();
  return useQuery<JobCompany>({ queryKey: keys.companies.job(jobId), queryFn: () => api.jobCompany(jobId) });
}

function useCompanyMutation<TData, TVars>(fn: (vars: TVars) => Promise<TData>, opts?: MutationOpts<TData, TVars>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.companies.all });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useWatchCompany(opts?: MutationOpts<WatchedCompany, { address: string } | { job_id: string }>) {
  const api = useApi();
  return useCompanyMutation((by) => api.watchCompany(by), opts);
}

export function useRenameCompany(opts?: MutationOpts<WatchedCompany, { id: string; name: string }>) {
  const api = useApi();
  return useCompanyMutation(({ id, name }) => api.renameCompany(id, name), opts);
}

export function useUnwatchCompany(opts?: MutationOpts<void, string>) {
  const api = useApi();
  return useCompanyMutation((id) => api.unwatchCompany(id), opts);
}
