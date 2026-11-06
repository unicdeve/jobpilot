import { skipToken, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { Application, ApplicationState, ApplicationStateTransition } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

export function useApplications(candidateId: string | null) {
  const api = useApi();
  return useQuery<Application[]>({
    queryKey: keys.applications.list(candidateId ?? ""),
    queryFn: candidateId !== null ? () => api.listApplications(candidateId) : skipToken,
  });
}

export function useApplication(id: string | undefined) {
  const api = useApi();
  return useQuery<Application>({
    queryKey: keys.applications.detail(id ?? ""),
    queryFn: id ? () => api.getApplication(id) : skipToken,
  });
}

export function useApplicationHistory(id: string | undefined) {
  const api = useApi();
  return useQuery<ApplicationStateTransition[]>({
    queryKey: keys.applications.history(id ?? ""),
    queryFn: id ? () => api.listApplicationHistory(id) : skipToken,
  });
}

/// useAllowedTransitions asks the daemon which states an application may
/// move to next. The rule table lives in Go and is enforced there; fetching
/// it rather than restating it here is what keeps the buttons on screen and
/// the transitions the server accepts from drifting apart.
export function useAllowedTransitions(id: string | undefined) {
  const api = useApi();
  return useQuery<ApplicationState[]>({
    queryKey: keys.applications.transitions(id ?? ""),
    queryFn: id ? () => api.listAllowedTransitions(id) : skipToken,
  });
}

interface CreateApplicationVars {
  jobId: string;
  candidateId: string;
  resumeId: string;
}

export function useCreateApplication(opts?: MutationOpts<Application, CreateApplicationVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, candidateId, resumeId }: CreateApplicationVars) =>
      api.createApplication(jobId, candidateId, resumeId),
    ...opts,
    onSuccess: (application, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.list(variables.candidateId) });
      opts?.onSuccess?.(application, variables, ...rest);
    },
  });
}

interface TransitionVars {
  id: string;
  toState: ApplicationState;
  reason: string;
}

export function useTransitionApplication(opts?: MutationOpts<Application, TransitionVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, toState, reason }: TransitionVars) => api.transitionApplication(id, toState, reason),
    ...opts,
    onSuccess: (application, variables, ...rest) => {
      queryClient.setQueryData(keys.applications.detail(variables.id), application);
      // A transition appends to the durable history and changes the state
      // every application list renders.
      void queryClient.invalidateQueries({ queryKey: keys.applications.history(variables.id) });
      void queryClient.invalidateQueries({ queryKey: keys.applications.all });
      opts?.onSuccess?.(application, variables, ...rest);
    },
  });
}

interface SetApplyUrlVars {
  applicationId: string;
  url: string;
}

export function useSetApplyUrl(opts?: MutationOpts<Application, SetApplyUrlVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ applicationId, url }: SetApplyUrlVars) => api.setApplyUrl(applicationId, url),
    ...opts,
    onSuccess: (application, variables, ...rest) => {
      queryClient.setQueryData(keys.applications.detail(variables.applicationId), application);
      opts?.onSuccess?.(application, variables, ...rest);
    },
  });
}
