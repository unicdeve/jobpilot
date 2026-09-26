import { useMutationState } from "@tanstack/react-query";
import { LoaderCircle } from "lucide-react";
import { useDaemon } from "../lib/daemon";
import { discoverySummary } from "../lib/format";
import { keys, useJobSources, useRunSavedSearch, type DiscoveryRun } from "../lib/queries";
import { ToastActivity, useToast } from "../lib/toast";

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

export interface PendingDiscovery {
  search: DiscoveryRun;
  /// queued is a run waiting for the one before it to finish.
  queued: boolean;
}

/// useDiscoveryRuns lists the discovery runs not finished yet, from every
/// screen, in the order they were asked.
export function useDiscoveryRuns(): PendingDiscovery[] {
  return useMutationState({
    filters: { mutationKey: keys.discovery, status: "pending" },
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
      <LoaderCircle className="mt-px size-4 shrink-0 animate-spin text-muted-foreground" />
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
