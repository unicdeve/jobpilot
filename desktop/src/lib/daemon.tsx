import { useQueryClient } from "@tanstack/react-query";
import { invoke } from "@tauri-apps/api/core";
import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { createApiClient, type ApiClient } from "./api";

interface DaemonState {
  status: "connecting" | "ready" | "error";
  client: ApiClient | null;
  error: string | null;
  retry: () => void;
}

const DaemonContext = createContext<DaemonState | null>(null);

/// DaemonProvider ensures the jobpilot daemon is running (starting it if
/// necessary, via the Rust ensure_daemon command in
/// desktop/src-tauri/src/daemon.rs) and provides an ApiClient pointed at
/// its resolved port to the rest of the app.
export function DaemonProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<Omit<DaemonState, "retry">>({
    status: "connecting",
    client: null,
    error: null,
  });
  const [attempt, setAttempt] = useState(0);
  const queryClient = useQueryClient();

  useEffect(() => {
    let cancelled = false;
    setState({ status: "connecting", client: null, error: null });

    invoke<number>("ensure_daemon")
      .then((port) => {
        if (cancelled) return;
        // A reconnect can resolve to a different daemon on a different port,
        // whose database is not necessarily the one the cache was filled
        // from. Start that connection with an empty cache rather than
        // showing the previous daemon's data.
        if (attempt > 0) queryClient.clear();
        setState({ status: "ready", client: createApiClient(port), error: null });
      })
      .catch((err) => {
        if (cancelled) return;
        setState({ status: "error", client: null, error: String(err) });
      });

    return () => {
      cancelled = true;
    };
  }, [attempt, queryClient]);

  return (
    <DaemonContext.Provider value={{ ...state, retry: () => setAttempt((a) => a + 1) }}>
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
