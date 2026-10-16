import { useQuery } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { SearchTemplateGroup } from "../types";
import { keys } from "./keys";

/// useSearchTemplates loads the catalog of ready-made searches. The catalog
/// is static Go data served by the daemon, so it never changes within a
/// session and is cached for the life of one. Importing one is a saved
/// search change (useImportSearchTemplates).
export function useSearchTemplates() {
  const api = useApi();
  return useQuery<SearchTemplateGroup[]>({
    queryKey: keys.searchTemplates,
    queryFn: () => api.listSearchTemplates(),
    staleTime: Infinity,
  });
}
