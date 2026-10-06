import { cn } from "cn";
import type { ReactNode } from "react";

/// Page is a screen that scrolls as a whole: a toolbar that stays put while
/// the body scrolls beneath it, the way a native window's toolbar does.
///
/// width caps the body for reading: "default" suits forms and detail, "wide"
/// suits lists, and "full" is for screens that lay out their own columns.
export function Page({
  title,
  actions,
  leading,
  children,
  width = "default",
  className,
}: {
  title: ReactNode;
  actions?: ReactNode;
  /// leading sits before the title: a back button on a narrow split view.
  leading?: ReactNode;
  children: ReactNode;
  width?: "default" | "wide" | "full";
  className?: string;
}) {
  return (
    <div className="flex min-h-full flex-col">
      <Toolbar title={title} actions={actions} leading={leading} />
      <div
        className={cn(
          "w-full px-6 pt-5 pb-10",
          width === "default" && "max-w-4xl",
          width === "wide" && "max-w-6xl",
          className,
        )}
      >
        {children}
      </div>
    </div>
  );
}

/// Toolbar is a window toolbar: the screen's title and its actions. It is a
/// drag region, so the window moves when it is dragged, as a title bar would.
export function Toolbar({
  title,
  actions,
  leading,
  className,
}: {
  title?: ReactNode;
  actions?: ReactNode;
  leading?: ReactNode;
  className?: string;
}) {
  return (
    <header
      data-tauri-drag-region
      className={cn(
        "sticky top-0 z-20 flex h-13 shrink-0 items-center gap-3 border-b border-border bg-background/85 px-6 backdrop-blur-md",
        className,
      )}
    >
      {leading}
      <h1 data-tauri-drag-region className="min-w-0 flex-1 truncate text-[15px] font-semibold">
        {title}
      </h1>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </header>
  );
}

/// Section is a titled group within a page body: a heading with an optional
/// description and actions, then its content. It replaces a card per
/// section, which is what made every screen read as a web page of boxes.
export function Section({
  title,
  description,
  actions,
  children,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("mt-8 first:mt-0", className)}>
      <div className="mb-3 flex items-end justify-between gap-4">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold">{title}</h2>
          {description && <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
      {children}
    </section>
  );
}

/// Panel is the one bordered surface: a grouped list, a form, a block of
/// settings. Content inside it is divided by hairlines rather than nested
/// boxes.
export function Panel({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("overflow-hidden rounded-lg border border-border bg-card", className)}>{children}</div>;
}

/// DetailSection is a titled part of a detail pane, divided from the one
/// above by a hairline. Its actions sit level with its title.
export function DetailSection({
  title,
  description,
  actions,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="mt-8 border-t border-border pt-5">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold">{title}</h3>
          {description && <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
      {children}
    </section>
  );
}
