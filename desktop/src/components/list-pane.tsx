import { cn } from "cn";
import type { ReactNode } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { Toolbar } from "./page";
import { splitBackClass } from "./split-view";

/// ListPane is the list side of a split view: a toolbar with the screen's
/// title and actions, optional controls beneath it, and the list, which
/// scrolls on its own so the toolbar stays put.
export function ListPane({
  title,
  actions,
  controls,
  children,
}: {
  title: ReactNode;
  actions?: ReactNode;
  controls?: ReactNode;
  children: ReactNode;
}) {
  return (
    <>
      <header
        data-tauri-drag-region
        className="flex h-13 shrink-0 items-center gap-2 border-b border-border px-4"
      >
        <h1 data-tauri-drag-region className="min-w-0 flex-1 truncate text-[15px] font-semibold">
          {title}
        </h1>
        {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
      </header>
      {controls && <div className="flex shrink-0 flex-col gap-2 border-b border-border px-3 py-2.5">{controls}</div>}
      <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
    </>
  );
}

/// ListItem is one selectable row. It keeps the list's query string when
/// followed, so a filtered list stays filtered while an item is open.
export function ListItem({ to, children }: { to: string; children: ReactNode }) {
  const { search } = useLocation();
  return (
    <NavLink
      to={{ pathname: to, search }}
      className={({ isActive }) =>
        cn(
          "block border-b border-border px-4 py-2.5 outline-none last:border-b-0 focus-visible:bg-accent",
          isActive ? "bg-primary/10 dark:bg-primary/15" : "hover:bg-accent/60",
        )
      }
    >
      {children}
    </NavLink>
  );
}

/// ListNote is a line of status text in a list: a count, an empty result, a
/// failure.
export function ListNote({ children, tone = "muted" }: { children: ReactNode; tone?: "muted" | "error" }) {
  return (
    <p className={cn("px-4 py-3 text-xs", tone === "error" ? "text-destructive" : "text-muted-foreground")}>
      {children}
    </p>
  );
}

/// NoSelection fills the detail pane when nothing is selected.
export function NoSelection({ icon, title, description }: { icon?: ReactNode; title: string; description?: string }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 px-8 text-center">
      {icon && <div className="text-muted-foreground/60 [&_svg]:size-10 [&_svg]:stroke-[1.25]">{icon}</div>}
      <p className="text-sm font-medium text-subtle-foreground">{title}</p>
      {description && <p className="max-w-xs text-sm text-muted-foreground">{description}</p>}
    </div>
  );
}

/// RecordGone is the detail side of a split view when the record it names
/// does not exist: deleted since its page was last open, or a stale link.
/// It reads as the empty selection does, since that is what is left, with
/// the way back the detail offers when it is shown on its own.
export function RecordGone({
  back,
  icon,
  title,
  description,
}: {
  back: ReactNode;
  icon?: ReactNode;
  title: string;
  description?: string;
}) {
  return (
    <div className="flex h-full flex-col">
      <Toolbar className={cn("px-5", splitBackClass)} leading={back} />
      <div className="min-h-0 flex-1">
        <NoSelection icon={icon} title={title} description={description} />
      </div>
    </div>
  );
}
