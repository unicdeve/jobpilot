import { LoaderCircle } from "lucide-react";
import { errorMessage } from "../lib/api";

/// Loading is the one way a screen says it is waiting: quiet, and in the
/// place its content will appear.
export function Loading({ label = "Loading…" }: { label?: string }) {
  return (
    <p className="flex items-center gap-2 px-6 py-5 text-sm text-muted-foreground">
      <LoaderCircle className="size-4 animate-spin" />
      {label}
    </p>
  );
}

export function ErrorLine({ error }: { error: unknown }) {
  return <p className="px-6 py-5 text-sm text-destructive">{errorMessage(error)}</p>;
}
