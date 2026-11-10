import { useQuery } from "@tanstack/react-query";
import { useApi } from "../daemon";
import { appVersion } from "../platform";
import { keys } from "./keys";

interface Versions {
  /// app is null in a browser, whose page the daemon served.
  app: string | null;
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
      const [app, health] = await Promise.all([appVersion(), api.health()]);
      return { app, daemon: health.version };
    },
  });
}
