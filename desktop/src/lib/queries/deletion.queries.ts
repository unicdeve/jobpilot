import { type Query, type QueryClient, skipToken, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { DeletionReach, DeletionResult, DeletionTarget } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useDeletionPreview loads what deleting target would take with it, each
/// time it is asked for: a preview is only worth showing if it is current,
/// so it is never cached. target null asks for nothing.
export function useDeletionPreview(target: DeletionTarget | null, id?: string) {
  const api = useApi();
  return useQuery<DeletionReach>({
    queryKey: keys.deletion.preview(target, id ?? ""),
    queryFn: target !== null ? () => api.previewDeletion(target, id) : skipToken,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
}

interface DeletionRequest {
  target: DeletionTarget;
  id?: string;
  token: string;
}

/// useDeleteRecords carries out a confirmed deletion. What one reaches is
/// not known to the client ahead of time — a job takes its applications,
/// the profile nearly everything — so every query is refreshed after it,
/// and after "delete all data" the cache is emptied instead.
///
/// The deleted record's own queries are the exception. The screen showing
/// it may still be mounted while it navigates away, and refreshed then, it
/// would ask the daemon for a record that no longer exists. They are left
/// as they are, and dropped once nothing shows them (dropWhenUnobserved).
export function useDeleteRecords(opts?: MutationOpts<DeletionResult, DeletionRequest>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ target, id, token }: DeletionRequest) => api.deleteRecords(target, id, token),
    ...opts,
    onSuccess: (result, variables, ...rest) => {
      if (variables.target === "everything") {
        opts?.onSuccess?.(result, variables, ...rest);
        queryClient.clear();
        return;
      }
      const gone = dropWhenUnobserved(queryClient, deletedRecordKeys(variables));
      void queryClient.invalidateQueries({ predicate: (query) => !gone.has(query.queryHash) });
      opts?.onSuccess?.(result, variables, ...rest);
    },
  });
}

/// dropWhenUnobserved removes the queries under keys from the cache as soon
/// as no component observes them, now or when the last one unmounts, and
/// returns their hashes.
function dropWhenUnobserved(queryClient: QueryClient, queryKeys: readonly (readonly unknown[])[]): Set<string> {
  const cache = queryClient.getQueryCache();
  const pending = new Map<string, Query>();
  for (const queryKey of queryKeys) {
    for (const query of cache.findAll({ queryKey })) pending.set(query.queryHash, query);
  }
  const hashes = new Set(pending.keys());

  const dropIfUnobserved = (query: Query) => {
    if (query.getObserversCount() > 0) return;
    cache.remove(query);
    pending.delete(query.queryHash);
  };
  pending.forEach(dropIfUnobserved);
  if (pending.size > 0) {
    const unsubscribe = cache.subscribe((event) => {
      if (event.type === "observerRemoved" && pending.has(event.query.queryHash)) dropIfUnobserved(event.query);
      if (pending.size === 0) unsubscribe();
    });
  }
  return hashes;
}

/// deletedRecordKeys are the queries that read the deleted record itself,
/// which would fail if they were asked again.
function deletedRecordKeys({ target, id }: DeletionRequest): readonly (readonly unknown[])[] {
  const previews = [keys.deletion.all] as const;
  if (!id) return previews;
  switch (target) {
    case "job":
      return [...previews, keys.jobs.detail(id), keys.matches.job(id), keys.companies.job(id)];
    case "application":
      return [
        ...previews,
        keys.applications.detail(id),
        keys.applications.history(id),
        keys.applications.transitions(id),
        keys.applications.coverLetters(id),
        keys.applications.questions(id),
      ];
    default:
      return previews;
  }
}
