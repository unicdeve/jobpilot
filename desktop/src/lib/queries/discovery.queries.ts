import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { useApi } from "../daemon";
import type { DiscoveryResult, DiscoverySettings, SavedSearch, SourceInfo } from "../types";
import { keys, mutationKeys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useJobSources names the sources a search asks. The daemon registers
/// them as it starts, so they do not change while it runs.
export function useJobSources() {
  const api = useApi();
  return useQuery<SourceInfo[]>({
    queryKey: keys.sources,
    queryFn: () => api.listSources(),
    staleTime: Infinity,
  });
}

/// DiscoveryRun is the saved search a discovery run uses. It carries the
/// name as well as the ID, so whatever shows the run can say which search
/// it is without looking it up.
export type DiscoveryRun = Pick<SavedSearch, "id" | "name">;

/// useRunSavedSearch discovers jobs with a saved search's criteria and
/// stores whatever matched, so every cached job list is then stale.
///
/// Runs share one scope, so they go one at a time in the order asked, from
/// whichever screen asked: two runs at once would put the same sources'
/// traffic and the same postings' writes side by side for no gain. A run
/// outlives the screen that started it.
export function useRunSavedSearch(opts?: MutationOpts<DiscoveryResult, DiscoveryRun>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: mutationKeys.discovery,
    scope: { id: "discovery" },
    mutationFn: (search: DiscoveryRun) => api.runSavedSearch(search.id),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.jobs.all });
      // The search now says when it last ran.
      void queryClient.invalidateQueries({ queryKey: keys.savedSearches });
      opts?.onSuccess?.(...args);
    },
    onSettled: (...args) => {
      // A search checks every watched company, whether or not it found jobs.
      void queryClient.invalidateQueries({ queryKey: keys.companies.list });
      opts?.onSettled?.(...args);
    },
  });
}

/// useDiscoverySettings loads how discovery behaves without being asked.
export function useDiscoverySettings() {
  const api = useApi();
  return useQuery<DiscoverySettings>({
    queryKey: keys.discoverySettings,
    queryFn: () => api.getDiscoverySettings(),
  });
}

export function useSaveDiscoverySettings(opts?: MutationOpts<DiscoverySettings, DiscoverySettings>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (settings: DiscoverySettings) => api.saveDiscoverySettings(settings),
    ...opts,
    onSuccess: (saved, ...rest) => {
      queryClient.setQueryData(keys.discoverySettings, saved);
      opts?.onSuccess?.(saved, ...rest);
    },
  });
}

/// useStaleSavedSearches returns a function listing the saved searches a
/// launch refresh runs: those gone stale, as the daemon decides, or none
/// when refreshing on launch is turned off. The settings it reads are
/// cached as useDiscoverySettings caches them.
export function useStaleSavedSearches(): () => Promise<SavedSearch[]> {
  const api = useApi();
  const queryClient = useQueryClient();
  return useCallback(async () => {
    const settings = await queryClient.fetchQuery({
      queryKey: keys.discoverySettings,
      queryFn: () => api.getDiscoverySettings(),
    });
    return settings.refresh_on_launch ? api.staleSavedSearches() : [];
  }, [api, queryClient]);
}
