import { useMutationState } from "@tanstack/react-query";
import { useEffect } from "react";
import { ApiError, errorMessage } from "../lib/api";
import { useDaemon } from "../lib/daemon";
import { discoverySummary, type RefreshFailure, refreshSummary } from "../lib/format";
import {
  type DiscoveryRun,
  useJobSources,
  useRunSavedSearch,
  useStaleSavedSearches,
} from "../lib/queries/discovery.queries";
import { mutationKeys } from "../lib/queries/keys";
import { ToastActivity, useToast } from "../lib/toast";
import type { SavedSearch } from "../lib/types";
import { Orb } from "./thinking";

/// useDiscover runs a saved search against the job sources and says how it
/// went once it has. Runs are queued and outlive the screen that started
/// them (useRunSavedSearch), so what one found is said wherever the user
/// is by then.
export function useDiscover() {
  const { notify } = useToast();
  return useRunSavedSearch({
    onSuccess: (result, search) => {
      const { message, complete } = discoverySummary(result, search.name);
      notify(message, complete ? "success" : "warning");
    },
    onError: (err, search) => notify(`Couldn’t search for “${search.name}”: ${err.message}`),
  });
}

interface PendingDiscovery {
  search: DiscoveryRun;
  /// queued is a run waiting for the one before it to finish.
  queued: boolean;
}

/// useDiscoveryRuns lists the discovery runs not finished yet, from every
/// screen, in the order they were asked.
export function useDiscoveryRuns(): PendingDiscovery[] {
  return useMutationState({
    filters: { mutationKey: mutationKeys.discovery, status: "pending" },
    select: (mutation) => ({
      search: mutation.state.variables as DiscoveryRun,
      queued: mutation.state.isPaused,
    }),
  });
}

/// DiscoveryActivity shows a search while it runs. A search asks every
/// source and waits for the slowest, so it can take a while; without this,
/// that wait looked like a result — a first run after setup showed a
/// dashboard of zeros.
export function DiscoveryActivity() {
  const { client } = useDaemon();
  const runs = useDiscoveryRuns();
  if (!client || runs.length === 0) return null;
  return <Searching runs={runs} />;
}

function Searching({ runs }: { runs: PendingDiscovery[] }) {
  const { data: sources } = useJobSources();
  const current = runs.find((run) => !run.queued) ?? runs[0];
  const after = runs.length - 1;

  return (
    <ToastActivity>
      <Orb state="searching" className="-mt-px" />
      <div className="min-w-0">
        <p className="font-medium" title={sources?.map((source) => source.name).join(", ")}>
          {sources
            ? `Searching ${sources.length} job ${sources.length === 1 ? "source" : "sources"}…`
            : "Searching your job sources…"}
        </p>
        {/* A long search name gives way; the count of what is queued does not. */}
        <p className="flex text-muted-foreground">
          <span className="truncate">For “{current.search.name}”</span>
          {after > 0 && <span className="shrink-0 whitespace-pre"> · {after} more queued</span>}
        </p>
      </div>
    </ToastActivity>
  );
}

/// refreshed is whether this launch has refreshed already. It is kept
/// outside React on purpose: it is once per launch, not per mount, so a
/// reconnect to a restarted daemon or StrictMode running an effect twice
/// does not search again.
let refreshed = false;

/// LaunchRefresh runs the saved searches gone stale when the app opens —
/// never run, or not in the last six hours — so the jobs are current
/// without the user asking (docs/02_features/04_job_discovery.md AC-009).
/// The daemon decides which are stale; the runs go through the same queue
/// as Discover's, and say what they found once, together.
export function LaunchRefresh() {
  const { client } = useDaemon();
  return client ? <Refresh /> : null;
}

function Refresh() {
  const staleSavedSearches = useStaleSavedSearches();
  const { notify } = useToast();
  const { mutateAsync } = useRunSavedSearch();

  useEffect(() => {
    if (refreshed) return;
    refreshed = true;

    void (async () => {
      let stale: SavedSearch[];
      try {
        stale = await staleSavedSearches();
      } catch {
        // Not knowing what is stale is not worth a message: nothing ran,
        // and Discover still works.
        return;
      }
      if (stale.length === 0) return;

      const settled = await Promise.allSettled(
        stale.map((search) => mutateAsync({ id: search.id, name: search.name })),
      );
      const results = settled.flatMap((outcome) => (outcome.status === "fulfilled" ? [outcome.value] : []));
      const failures: RefreshFailure[] = settled.flatMap((outcome) =>
        outcome.status === "rejected"
          ? [
              {
                offline: outcome.reason instanceof ApiError && outcome.reason.status === 503,
                message: errorMessage(outcome.reason),
              },
            ]
          : [],
      );
      const { message, complete } = refreshSummary(results, failures);
      notify(message, complete ? "success" : "warning");
    })();
  }, [staleSavedSearches, mutateAsync, notify]);

  return null;
}
