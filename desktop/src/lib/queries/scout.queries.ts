import { type QueryClient, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import type { ScoutInput } from "../api";
import { useApi } from "../daemon";
import type { AgentRun, Scout, ScoutRunReport } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// invalidateRunResults refetches what a scout run changes: the scouts and
/// their runs, the applications it made for review, the activity that
/// records it and the notification that says so.
function invalidateRunResults(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: keys.scouts.all });
  void queryClient.invalidateQueries({ queryKey: keys.applications.all });
  void queryClient.invalidateQueries({ queryKey: keys.activity.all });
  void queryClient.invalidateQueries({ queryKey: keys.notifications });
}

/// useScouts loads the standing instructions that shortlist jobs for
/// review. An empty list is where every installation starts.
///
/// While a scout is running or waiting its turn, or poll is set because a
/// run was just asked for, the list is read every two seconds, so a run
/// the schedule started shows, and so does its end. A run the schedule
/// started ends without anyone asking for it, so its end is noticed here
/// and what it made is read again.
export function useScouts(poll = false) {
  const api = useApi();
  const queryClient = useQueryClient();
  const query = useQuery<Scout[]>({
    queryKey: keys.scouts.list,
    queryFn: () => api.listScouts(),
    refetchInterval: (query) => (poll || query.state.data?.some((scout) => scout.activity) ? 2000 : false),
  });

  const busy = query.data?.some((scout) => scout.activity) ?? false;
  const wasBusy = useRef(busy);
  useEffect(() => {
    if (wasBusy.current && !busy) invalidateRunResults(queryClient);
    wasBusy.current = busy;
  }, [busy, queryClient]);

  return query;
}

export function useScoutRuns(limit = 10) {
  const api = useApi();
  return useQuery<AgentRun[]>({
    queryKey: keys.scouts.runs(limit),
    queryFn: () => api.listScoutRuns(limit),
  });
}

function useScoutMutation<TData, TVars>(fn: (vars: TVars) => Promise<TData>, opts?: MutationOpts<TData, TVars>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.scouts.list });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useCreateScout(opts?: MutationOpts<Scout, ScoutInput>) {
  const api = useApi();
  return useScoutMutation((scout) => api.createScout(scout), opts);
}

export function useUpdateScout(opts?: MutationOpts<Scout, { id: string; scout: ScoutInput }>) {
  const api = useApi();
  return useScoutMutation(({ id, scout }) => api.updateScout(id, scout), opts);
}

export function useDeleteScout(opts?: MutationOpts<void, string>) {
  const api = useApi();
  return useScoutMutation((id) => api.deleteScout(id), opts);
}

/// useRunScout runs a scout once.
export function useRunScout(opts?: MutationOpts<ScoutRunReport, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.runScout(id),
    ...opts,
    // On settle, not success: a run that fails partway is still recorded,
    // and may already have created applications (AC-006).
    onSettled: (...args) => {
      invalidateRunResults(queryClient);
      opts?.onSettled?.(...args);
    },
  });
}
