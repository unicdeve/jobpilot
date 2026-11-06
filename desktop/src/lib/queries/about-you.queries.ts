import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { AboutYou, AboutYouUpdate, PersonalAnswer } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useAboutYou loads what the candidate has said about themselves. With no
/// profile yet there is nothing to load, which is null rather than an
/// error, as it is for the profile.
export function useAboutYou() {
  const api = useApi();
  return useQuery<AboutYou | null>({
    queryKey: keys.aboutYou,
    queryFn: () => api.getAboutYou(),
  });
}

export function useSaveAboutYou(opts?: MutationOpts<AboutYou, AboutYouUpdate>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (update: AboutYouUpdate) => api.saveAboutYou(update),
    ...opts,
    onSuccess: (saved, ...rest) => {
      queryClient.setQueryData(keys.aboutYou, saved);
      opts?.onSuccess?.(saved, ...rest);
    },
  });
}

export function useDeleteKeptAnswer(opts?: MutationOpts<void, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteKeptAnswer(id),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.aboutYou });
      opts?.onSuccess?.(...args);
    },
  });
}

/// useKeepAnswer keeps an answer given on one application, so a similar
/// question on another is offered it.
export function useKeepAnswer(opts?: MutationOpts<PersonalAnswer, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (questionId: string) => api.keepAnswer(questionId),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.aboutYou });
      // A kept answer is suggested for similar questions on every
      // application, so their suggestions are read again.
      void queryClient.invalidateQueries({ queryKey: keys.applications.all });
      opts?.onSuccess?.(...args);
    },
  });
}
