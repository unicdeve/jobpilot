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

interface CreateCoverLetterVars {
  applicationId: string;
  content: string;
}

/// useCreateCoverLetter saves the candidate's letter as a new version.
export function useCreateCoverLetter(opts?: MutationOpts<CoverLetter, CreateCoverLetterVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ applicationId, content }: CreateCoverLetterVars) => api.createCoverLetter(applicationId, content),
    ...opts,
    onSuccess: (letter, variables, ...rest) => {
      // A new version is added rather than replacing one, and the
      // application records it as current, so both are refetched.
      void queryClient.invalidateQueries({ queryKey: keys.applications.coverLetters(variables.applicationId) });
      void queryClient.invalidateQueries({ queryKey: keys.applications.detail(variables.applicationId) });
      opts?.onSuccess?.(letter, variables, ...rest);
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
