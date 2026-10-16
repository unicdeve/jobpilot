import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { PromptGuidance, PromptGuidanceList, PromptKey } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// usePromptGuidance loads the prompts the user may give guidance to
/// (ADR-0031).
export function usePromptGuidance() {
  const api = useApi();
  return useQuery<PromptGuidanceList>({
    queryKey: keys.promptGuidance,
    queryFn: () => api.listPromptGuidance(),
  });
}

/// replacePrompt writes one prompt, as the daemon returned it, into the
/// cached list.
function replacePrompt(list: PromptGuidanceList | undefined, prompt: PromptGuidance) {
  return list && { ...list, prompts: list.prompts.map((p) => (p.key === prompt.key ? prompt : p)) };
}

/// usePromptMutation runs fn and writes the prompt it returns into the
/// cached list, which needs no request.
function usePromptMutation<TVars>(
  fn: (vars: TVars) => Promise<PromptGuidance>,
  opts?: MutationOpts<PromptGuidance, TVars>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    ...opts,
    onSuccess: (prompt, ...rest) => {
      queryClient.setQueryData<PromptGuidanceList>(keys.promptGuidance, (list) => replacePrompt(list, prompt));
      opts?.onSuccess?.(prompt, ...rest);
    },
  });
}

export function useSavePromptGuidance(opts?: MutationOpts<PromptGuidance, { key: PromptKey; text: string }>) {
  const api = useApi();
  return usePromptMutation(({ key, text }) => api.savePromptGuidance(key, text), opts);
}

/// useResetPromptGuidance puts a prompt back on its default.
export function useResetPromptGuidance(opts?: MutationOpts<PromptGuidance, PromptKey>) {
  const api = useApi();
  return usePromptMutation((key) => api.resetPromptGuidance(key), opts);
}
