import { skipToken, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import {
  type AgentRun,
  type AgentRunWithEvents,
  type Application,
  type ApplicationMode,
  type ApplicationState,
  type ApplicationStateTransition,
  isRunInProgress,
} from "../types";
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

/// useAgentRuns loads the browser runs that prepared one application. A run
/// goes on in the daemon after the request that started it, so while one is
/// in progress the list is read again every two seconds, which is how the
/// screen learns that it is waiting for the user, or has finished.
export function useAgentRuns(id: string | undefined) {
  const api = useApi();
  return useQuery<AgentRunWithEvents[]>({
    queryKey: keys.applications.agentRuns(id ?? ""),
    queryFn: id ? () => api.listAgentRuns(id) : skipToken,
    refetchInterval: (query) => (query.state.data?.some(({ run }) => isRunInProgress(run.status)) ? 2000 : false),
  });
}

interface CreateApplicationVars {
  jobId: string;
  candidateId: string;
  resumeId: string;
  mode: ApplicationMode;
}

export function useCreateApplication(opts?: MutationOpts<Application, CreateApplicationVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, candidateId, resumeId, mode }: CreateApplicationVars) =>
      api.createApplication(jobId, candidateId, resumeId, mode),
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

export function usePrepareInBrowser(opts?: MutationOpts<AgentRun, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (applicationId: string) => api.prepareInBrowser(applicationId),
    ...opts,
    // Starting moves the application into PREPARING, and the new run is
    // what useAgentRuns then follows. keys.applications.all covers the
    // application, its runs and its history.
    onSettled: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.all });
      void queryClient.invalidateQueries({ queryKey: keys.activity.all });
      opts?.onSettled?.(...args);
    },
  });
}

export function useStopAgentRun(opts?: MutationOpts<void, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (runId: string) => api.stopAgentRun(runId),
    ...opts,
    onSettled: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.all });
      void queryClient.invalidateQueries({ queryKey: keys.activity.all });
      opts?.onSettled?.(...args);
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
