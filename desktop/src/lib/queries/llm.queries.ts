import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { LLMHealth, LLMSettings, SaveLLMSettingsResult } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

export function useLLMSettings() {
  const api = useApi();
  return useQuery<LLMSettings>({
    queryKey: keys.llmSettings,
    queryFn: () => api.getLLMSettings(),
  });
}

export function useSaveLLMSettings(opts?: MutationOpts<SaveLLMSettingsResult, LLMSettings>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (settings: LLMSettings) => api.saveLLMSettings(settings),
    ...opts,
    onSuccess: (result, ...rest) => {
      queryClient.setQueryData(keys.llmSettings, result.settings);
      opts?.onSuccess?.(result, ...rest);
    },
  });
}

/// useCheckLLMHealth asks the daemon whether the configured backend would
/// answer right now. It is a mutation rather than a query because it runs on
/// demand, immediately before work that needs the LLM — probing on every
/// render of a screen would mean pinging the backend for nothing.
export function useCheckLLMHealth(opts?: MutationOpts<LLMHealth, void>) {
  const api = useApi();
  return useMutation({
    mutationFn: () => api.checkLLMHealth(),
    ...opts,
  });
}
