import { useDaemon } from "../lib/daemon";
import { Loading } from "./status-line";
import { Button } from "./ui/button";

/// RequireDaemon holds back anything that talks to the daemon until the
/// connection is ready. useApi() throws when it is not, so a screen rendered
/// outside this gate takes the whole app down with it rather than failing
/// visibly — which is why first-run setup needs it as much as the main shell
/// does, despite living outside the sidebar.
export function RequireDaemon({ children }: { children: React.ReactNode }) {
  const { status, error, retry } = useDaemon();

  if (status === "connecting") {
    return <Loading label="Connecting to JobPilot…" />;
  }

  if (status === "error") {
    return (
      <div className="m-6 max-w-md space-y-3 rounded-lg border border-destructive/30 bg-destructive/8 p-4">
        <p className="text-sm font-medium text-destructive">Could not connect to the JobPilot daemon.</p>
        <p className="text-sm text-subtle-foreground select-text">{error}</p>
        <Button variant="outline" onClick={retry}>
          Retry
        </Button>
      </div>
    );
  }

  return <>{children}</>;
}
