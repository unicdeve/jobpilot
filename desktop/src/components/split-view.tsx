import { cn } from "cn";
import type { ReactNode } from "react";

/// SplitView is a list beside the item selected from it, as in Mail or
/// Finder: the list keeps its place and scroll while the detail changes.
///
/// A window too narrow for both shows one: the detail when something is
/// selected, the list otherwise, and the detail offers a way back. The
/// breakpoint is the width at which the detail pane would fall below a
/// comfortable reading measure.
export function SplitView({
  list,
  detail,
  hasSelection,
}: {
  list: ReactNode;
  detail: ReactNode;
  hasSelection: boolean;
}) {
  return (
    <div className="absolute inset-0 flex">
      <div
        className={cn(
          "min-h-0 w-full flex-col border-r border-border min-[1180px]:flex min-[1180px]:w-[400px] min-[1180px]:shrink-0",
          hasSelection ? "hidden" : "flex",
        )}
      >
        {list}
      </div>
      <div
        className={cn(
          "min-h-0 min-w-0 flex-1 overflow-y-auto min-[1180px]:block",
          hasSelection ? "block" : "hidden",
        )}
      >
        {detail}
      </div>
    </div>
  );
}

/// splitBackClass shows an element only when the split view is showing a
/// single pane, which is when a detail needs its own way back to the list.
export const splitBackClass = "min-[1180px]:hidden";
