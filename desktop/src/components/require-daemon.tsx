import { useDaemon } from "../lib/daemon";
import { Button } from "./ui/button";

/// RequireDaemon holds back anything that talks to the daemon until the
/// connection is ready. useApi() throws when it is not, so a screen rendered
/// outside this gate takes the whole app down with it rather than failing
/// visibly — which is why first-run setup needs it as much as the main shell
/// does, despite living outside the sidebar.
export function RequireDaemon({ children }: { children: React.ReactNode }) {
  const { status, error, retry } = useDaemon();

  if (status === "connecting") {
    return <p className="text-sm text-slate-500">Connecting to the jobpilot daemon…</p>;
  }

  if (status === "error") {
    return (
      <div className="max-w-md space-y-3 rounded-md border border-red-200 bg-red-50 p-4">
        <p className="text-sm font-medium text-red-800">Could not connect to the jobpilot daemon.</p>
        <p className="text-sm text-red-700">{error}</p>
        <Button variant="secondary" onClick={retry}>
          Retry
        </Button>
      </div>
    );
  }

  return <>{children}</>;
}
