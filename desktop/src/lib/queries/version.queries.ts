import { useQuery } from "@tanstack/react-query";
import { invoke } from "@tauri-apps/api/core";
import { useApi } from "../daemon";
import { keys } from "./keys";

interface Versions {
  app: string;
  daemon: string;
}

/// useVersions is this app's version and the daemon's, which can differ: an
/// app finds a newer daemon and uses it rather than replacing it
/// (ADR-0029 §4).
export function useVersions() {
  const api = useApi();
  return useQuery<Versions>({
    queryKey: keys.versions,
    queryFn: async () => {
      const [app, health] = await Promise.all([invoke<string>("app_version"), api.health()]);
      return { app, daemon: health.version };
    },
  });
}
