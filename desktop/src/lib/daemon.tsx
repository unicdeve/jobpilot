import { useQueryClient } from "@tanstack/react-query";
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { createContext, type ReactNode, useCallback, useContext, useEffect, useRef, useState } from "react";
import { type ApiClient, createApiClient } from "./api";

interface DaemonState {
  status: "connecting" | "ready" | "error";
  client: ApiClient | null;
  error: string | null;
  /// waiting is a connection held up by an older daemon this app is
  /// replacing, while it stops (ADR-0029 §4).
  waiting: boolean;
  retry: () => void;
}

const DaemonContext = createContext<DaemonState | null>(null);

/// DaemonProvider ensures the jobpilot daemon is running (starting it if
/// necessary, via the Rust ensure_daemon command in
/// desktop/src-tauri/src/daemon.rs) and provides an ApiClient pointed at
/// its resolved port to the rest of the app.
///
/// A daemon that stops answering is looked for again (find_daemon), which
/// waits for a daemon but never starts one: the one that went away may be
/// restarting — on another port, since each start takes a free one — and a
/// daemon started in that gap would be a second on the same database. Found
/// again, the app carries on where it was; not found, it says so and Retry
/// starts one.
export function DaemonProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<Omit<DaemonState, "retry" | "waiting">>({
    status: "connecting",
    client: null,
    error: null,
  });
  const [waiting, setWaiting] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const queryClient = useQueryClient();

  const port = useRef<number | null>(null);
  const finding = useRef(false);
  const refetchOnConnect = useRef(false);

  // Named, so a client created here reports to this same handler.
  const onUnreachable = useCallback(
    function onUnreachable() {
      if (finding.current || port.current === null) return;
      finding.current = true;
      invoke<number>("find_daemon")
        .then((found) => {
          // The cache is kept: the daemon is found through the discovery
          // file in its data directory, so whichever daemon this is serves
          // the same database. What was fetched from the one that went away
          // is only refetched.
          if (found === port.current) {
            void queryClient.invalidateQueries();
            return;
          }
          port.current = found;
          refetchOnConnect.current = true;
          setState({ status: "ready", client: createApiClient(found, onUnreachable), error: null });
        })
        .catch((err) => {
          port.current = null;
          setState({ status: "error", client: null, error: String(err) });
        })
        .finally(() => {
          finding.current = false;
        });
    },
    [queryClient],
  );

  // After the screens have rendered with the new client, so what they
  // refetch goes to it.
  useEffect(() => {
    if (state.client && refetchOnConnect.current) {
      refetchOnConnect.current = false;
      void queryClient.invalidateQueries();
    }
  }, [state.client, queryClient]);

  useEffect(() => {
    let cancelled = false;
    setState({ status: "connecting", client: null, error: null });
    setWaiting(false);
    const unlisten = listen("daemon-waiting", () => {
      if (!cancelled) setWaiting(true);
    });

    invoke<number>("ensure_daemon")
      .then((resolved) => {
        if (cancelled) return;
        // A reconnect can resolve to a different daemon on a different port,
        // whose database is not necessarily the one the cache was filled
        // from. Start that connection with an empty cache rather than
        // showing the previous daemon's data.
        if (attempt > 0) queryClient.clear();
        port.current = resolved;
        setState({ status: "ready", client: createApiClient(resolved, onUnreachable), error: null });
      })
      .catch((err) => {
        if (cancelled) return;
        setState({ status: "error", client: null, error: String(err) });
      })
      .finally(() => {
        if (!cancelled) setWaiting(false);
      });

    return () => {
      cancelled = true;
      void unlisten.then((stop) => stop());
    };
  }, [attempt, queryClient, onUnreachable]);

  return (
    <DaemonContext.Provider value={{ ...state, waiting, retry: () => setAttempt((a) => a + 1) }}>
      {children}
    </DaemonContext.Provider>
  );
}

/// useDaemon returns the current daemon connection state. Screens that need
/// the API client should prefer useApi(), which throws if not yet ready.
export function useDaemon(): DaemonState {
  const ctx = useContext(DaemonContext);
  if (!ctx) throw new Error("useDaemon must be used within a DaemonProvider");
  return ctx;
}

/// useApi returns the ready ApiClient. Only call this from screens rendered
/// inside <RequireDaemon>, which guarantees status === "ready".
export function useApi(): ApiClient {
  const { client } = useDaemon();
  if (!client) throw new Error("useApi called before the daemon connection was ready");
  return client;
}
