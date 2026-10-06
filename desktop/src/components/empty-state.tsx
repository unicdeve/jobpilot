import { cn } from "cn";
import { Link } from "react-router-dom";
import { Button } from "./ui/button";

/// fillsContentArea is the height of <main>'s visible area: the viewport
/// less the p-6 it applies top and bottom. A screen whose whole body is an
/// EmptyState uses it on its own root, so the empty state can centre in what
/// is left after any heading above it instead of overflowing past the fold.
export const fillsContentArea = "min-h-[calc(100vh-3rem)]";

/// EmptyState centres a short explanation and one action in the content
/// area, for screens that cannot show anything until something else exists.
///
/// It is centred rather than left-aligned because the whole screen is the
/// message: a line of grey text in the top-left corner of an otherwise blank
/// page reads as something failing to load, not as an invitation.
export function EmptyState({
  title,
  description,
  actionLabel,
  actionTo,
  className,
}: {
  title: string;
  description?: string;
  actionLabel?: string;
  actionTo?: string;
  /// Override the default full-area height when the empty state sits below
  /// a heading: pass "min-h-0 flex-1" inside a fillsContentArea column.
  className?: string;
}) {
  return (
    <div
      className={cn("flex flex-col items-center justify-center px-6 text-center", fillsContentArea, className)}
      data-slot="empty-state"
    >
      <h2 className="text-lg font-semibold text-foreground">{title}</h2>
      {description && <p className="mt-2 max-w-md text-sm text-muted-foreground">{description}</p>}
      {actionLabel && actionTo && <Button className="mt-5" render={<Link to={actionTo}>{actionLabel}</Link>} />}
    </div>
  );
}
