import { skipToken, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { CoverLetter } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useCoverLetters loads every version of one application's letter.
export function useCoverLetters(applicationId: string | undefined) {
  const api = useApi();
  return useQuery<CoverLetter[]>({
    queryKey: keys.applications.coverLetters(applicationId ?? ""),
    queryFn: applicationId ? () => api.listCoverLetterVersions(applicationId) : skipToken,
  });
}

export function useGenerateCoverLetter(opts?: MutationOpts<CoverLetter, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (applicationId: string) => api.generateCoverLetter(applicationId),
    ...opts,
    onSuccess: (letter, applicationId, ...rest) => {
      // Generating adds a version rather than replacing one, so the whole
      // version list is refetched — and the application, which records the
      // new version as current. Without it a rewrite kept showing the
      // letter it replaced until the application happened to refetch.
      void queryClient.invalidateQueries({ queryKey: keys.applications.coverLetters(applicationId) });
      void queryClient.invalidateQueries({ queryKey: keys.applications.detail(applicationId) });
      opts?.onSuccess?.(letter, applicationId, ...rest);
    },
  });
}

interface EditCoverLetterVars {
  coverLetterId: string;
  applicationId: string;
  content: string;
}

export function useEditCoverLetter(opts?: MutationOpts<CoverLetter, EditCoverLetterVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ coverLetterId, content }: EditCoverLetterVars) => api.editCoverLetter(coverLetterId, content),
    ...opts,
    onSuccess: (letter, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.coverLetters(variables.applicationId) });
      opts?.onSuccess?.(letter, variables, ...rest);
    },
  });
}
