import { skipToken, useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useApi } from "../daemon";
import { type AgentRunDetail, type AgentRunFilter, isRunInProgress } from "../types";
import { keys } from "./keys";

/// useInfiniteActivity pages through every agent run, newest first, the
/// same way useInfiniteJobs pages through stored jobs.
export function useInfiniteActivity(filter: AgentRunFilter, pageSize: number) {
  const api = useApi();
  return useInfiniteQuery({
    queryKey: keys.activity.infinite(filter, pageSize),
    queryFn: ({ pageParam }) => api.listActivity(filter, pageSize, pageParam),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((count, page) => count + page.runs.length, 0);
      return loaded < lastPage.total ? loaded : undefined;
    },
  });
}

/// useActivityRun loads one run with every step it recorded, and reads it
/// again every two seconds while it is in progress, so its steps appear as
/// it takes them.
export function useActivityRun(id: string | undefined) {
  const api = useApi();
  return useQuery<AgentRunDetail>({
    queryKey: keys.activity.detail(id ?? ""),
    queryFn: id ? () => api.getActivityRun(id) : skipToken,
    refetchInterval: (query) => (query.state.data && isRunInProgress(query.state.data.run.status) ? 2000 : false),
  });
}
