import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { Countries, SavedSearch, SearchCriteria } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useSavedSearches loads the named searches that drive discovery, default
/// first. An empty list is the state a new installation starts in, and the
/// state in which discovery cannot run at all.
export function useSavedSearches() {
  const api = useApi();
  return useQuery<SavedSearch[]>({
    queryKey: keys.savedSearches,
    queryFn: () => api.listSavedSearches(),
  });
}

/// useCountries lists the countries a search can work from, and the
/// profile's own. The list is the daemon's and does not change while it
/// runs; the profile's country is refreshed when the profile is saved.
export function useCountries() {
  const api = useApi();
  return useQuery<Countries>({
    queryKey: keys.countries,
    queryFn: () => api.listCountries(),
    staleTime: Infinity,
  });
}

/// useSavedSearchMutation runs fn and refetches the saved searches after
/// it. Every change refetches the whole list rather than patching one row:
/// deleting the default promotes another, and the daemon orders the list.
function useSavedSearchMutation<TData, TVars>(fn: (vars: TVars) => Promise<TData>, opts?: MutationOpts<TData, TVars>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.savedSearches });
      opts?.onSuccess?.(...args);
    },
  });
}

interface SavedSearchFields {
  name: string;
  criteria: SearchCriteria;
}

export function useCreateSavedSearch(opts?: MutationOpts<SavedSearch, SavedSearchFields>) {
  const api = useApi();
  return useSavedSearchMutation(({ name, criteria }) => api.createSavedSearch(name, criteria), opts);
}

export function useUpdateSavedSearch(opts?: MutationOpts<SavedSearch, SavedSearchFields & { id: string }>) {
  const api = useApi();
  return useSavedSearchMutation(({ id, name, criteria }) => api.updateSavedSearch(id, name, criteria), opts);
}

export function useDeleteSavedSearch(opts?: MutationOpts<void, string>) {
  const api = useApi();
  return useSavedSearchMutation((id) => api.deleteSavedSearch(id), opts);
}

export function useSetDefaultSavedSearch(opts?: MutationOpts<SavedSearch, string>) {
  const api = useApi();
  return useSavedSearchMutation((id) => api.setDefaultSavedSearch(id), opts);
}

/// useImportSearchTemplates adopts templates as ordinary saved searches,
/// each starting with the profile's country as the one the candidate works
/// from, which no template sets (ADR-0023 §4).
export function useImportSearchTemplates(opts?: MutationOpts<SavedSearch[], string[]>) {
  const api = useApi();
  const { data: countries } = useCountries();
  return useSavedSearchMutation(
    (templateIds) => api.importSearchTemplates(templateIds, countries?.profile_country),
    opts,
  );
}
