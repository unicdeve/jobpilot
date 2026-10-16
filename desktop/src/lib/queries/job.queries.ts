import {
  type QueryObserverResult,
  skipToken,
  useInfiniteQuery,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useMemo } from "react";
import { useApi } from "../daemon";
import { type StoredJobsFilter, since } from "../job-filters";
import type { Job, JobPage } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// A job's record changes only when the user rules it out or back in, which
/// replaces its cache entry directly, so it stays fresh far longer than a
/// list does.
const JOB_STALE_TIME = 5 * 60_000;

/// useJobCount reports how many jobs are stored locally. `total` describes
/// the whole stored list independently of the page around it, so asking for
/// a single row is the cheapest way to read it.
export function useJobCount() {
  const api = useApi();
  return useQuery({
    queryKey: keys.jobs.count,
    queryFn: () => api.listJobs({}, 1, 0),
    select: (page: JobPage) => page.total,
  });
}

/// useInfiniteJobs pages through stored jobs. Every page already loaded stays
/// in the cache, so returning to a list that was scrolled several pages deep
/// renders at the same height it was left at — which is what makes restoring
/// the scroll position meaningful rather than a jump to a half-built list.
export function useInfiniteJobs(filter: StoredJobsFilter, pageSize: number) {
  const api = useApi();
  // One instant per chosen window, so every page of a list counts back from
  // the same moment and none shifts between pages.
  const postedSince = useMemo(() => (filter.posted ? since(filter.posted) : undefined), [filter.posted]);
  return useInfiniteQuery({
    queryKey: keys.jobs.infinite(filter, pageSize),
    queryFn: ({ pageParam }) =>
      api.listJobs(
        {
          keywords: filter.keywords,
          source: filter.source,
          remote: filter.remote,
          postedSince,
          interest: filter.notInterested ? "not_interested" : undefined,
          workFrom: filter.workFrom,
          includeRelocation: filter.includeRelocation,
        },
        pageSize,
        pageParam,
      ),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      // The next offset is however many rows are already loaded; total counts
      // the whole filtered result, so this stops exactly at the end.
      const loaded = allPages.reduce((count, page) => count + page.jobs.length, 0);
      return loaded < lastPage.total ? loaded : undefined;
    },
  });
}

/// useJob loads one stored job.
export function useJob(id: string | undefined) {
  const api = useApi();
  return useQuery<Job>({
    queryKey: keys.jobs.detail(id ?? ""),
    queryFn: id ? () => api.getJob(id) : skipToken,
    staleTime: JOB_STALE_TIME,
  });
}

/// jobsByID is defined once, outside any component, so useQueries keeps the
/// map it returns until one of the jobs changes rather than building a new
/// one on every render.
function jobsByID(results: QueryObserverResult<Job>[]): Map<string, Job> {
  return new Map(results.flatMap((result) => (result.data ? [[result.data.id, result.data] as const] : [])));
}

/// useJobsByID loads several jobs at once, for screens that hold job IDs and
/// need something human-readable to show for them — a match or an
/// application knows only its job_id.
///
/// This is one request per job rather than a bulk endpoint. It is a local
/// SQLite read over loopback, the counts involved are however many jobs the
/// user has personally analysed or applied to, and every result is cached
/// under the same key useJob uses — so opening one of those jobs afterwards
/// needs no request at all. If these lists ever grow large enough for that
/// to bite, the fix is a bulk read on the daemon, not a cache here.
export function useJobsByID(ids: string[]): Map<string, Job> {
  const api = useApi();
  return useQueries({
    queries: [...new Set(ids)].map((id) => ({
      queryKey: keys.jobs.detail(id),
      queryFn: () => api.getJob(id),
      staleTime: JOB_STALE_TIME,
    })),
    combine: jobsByID,
  });
}

export function useImportJob(opts?: MutationOpts<Job, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (url: string) => api.importJob(url),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.jobs.all });
      opts?.onSuccess?.(...args);
    },
  });
}

/// useSetJobInterest rules a job out or back in. The job's own record
/// changes, so its cached copy is replaced, and every list it leaves or
/// rejoins is refreshed.
export function useSetJobInterest(opts?: MutationOpts<Job, { id: string; notInterested: boolean }>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, notInterested }: { id: string; notInterested: boolean }) =>
      api.setJobInterest(id, notInterested),
    ...opts,
    onSuccess: (job, variables, ...rest) => {
      queryClient.setQueryData(keys.jobs.detail(job.id), job);
      void queryClient.invalidateQueries({ queryKey: keys.jobs.all });
      void queryClient.invalidateQueries({ queryKey: keys.matches.all });
      opts?.onSuccess?.(job, variables, ...rest);
    },
  });
}
