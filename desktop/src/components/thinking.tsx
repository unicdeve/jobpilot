import type { ReactNode } from "react";
import { type OrbState, ThinkingOrb } from "thinking-orbs";
import { cn } from "../lib/utils";

/// Orb is the mark for JobPilot doing work on the user's behalf: the model
/// reading or writing, or a search across the job sources. On a local model
/// that can take a minute. A read from the daemon is the other kind of wait,
/// and keeps the quiet spinner in Loading (status-line.tsx), so the orb only
/// ever means "working for you".
///
/// The state is the verb: composing to write, solving to weigh a match,
/// searching to ask the job sources, working to read. The orb follows the
/// theme from the `dark` class, and holds still under reduced motion.
export function Orb({ state, className }: { state: OrbState; className?: string }) {
  // The words beside it say what is happening; the orb's own label would
  // only repeat them, less precisely.
  return <ThinkingOrb state={state} size={20} aria-hidden className={cn("shrink-0", className)} />;
}

/// Thinking is the line that says what JobPilot is doing, in the place its
/// result will appear.
export function Thinking({ state, children, className }: { state: OrbState; children: ReactNode; className?: string }) {
  return (
    <p role="status" className={cn("flex items-center gap-2 text-[13px] text-muted-foreground", className)}>
      <Orb state={state} />
      {children}
    </p>
  );
}
