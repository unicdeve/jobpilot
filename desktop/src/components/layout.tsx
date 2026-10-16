import { cn } from "cn";
import {
  Briefcase,
  FileText,
  History,
  House,
  type LucideIcon,
  Radar,
  Send,
  Settings as SettingsIcon,
  UserRound,
} from "lucide-react";
import { Activity, Suspense, useEffect, useRef, useState } from "react";
import { Link, type Location, Navigate, Routes, useLocation, useNavigate } from "react-router-dom";
import { useDaemon } from "../lib/daemon";
import { isMac, modLabel } from "../lib/platform";
import { PortalContainer } from "../lib/portal-container";
import { useApplications } from "../lib/queries/application.queries";
import { useLLMSettings } from "../lib/queries/llm.queries";
import { useCandidateId, useProfile } from "../lib/queries/profile.queries";
import { useSavedSearches } from "../lib/queries/saved-search.queries";
import { setupSkipped } from "../lib/setup-skip";
import { preloadSections, SECTIONS, type Section, sectionFor } from "../sections";
import { ErrorBoundary } from "./error-boundary";
import { RequireDaemon } from "./require-daemon";
import { Button } from "./ui/button";
import { WorkspaceSwitcher } from "./workspace-switcher";

interface NavItem {
  /// The section's root, as in SECTIONS.
  to: string;
  label: string;
  icon: LucideIcon;
}

/// Screens are grouped by what they are for: the work itself, what runs on
/// the user's behalf, and what JobPilot knows about them. The order is also
/// the order of the ⌘1–⌘7 shortcuts.
const navGroups: { label?: string; items: NavItem[] }[] = [
  {
    items: [
      { to: "/", label: "Dashboard", icon: House },
      { to: "/jobs", label: "Jobs", icon: Briefcase },
      { to: "/applications", label: "Applications", icon: Send },
    ],
  },
  {
    label: "Automation",
    items: [
      { to: "/scouts", label: "Scouts", icon: Radar },
      { to: "/activity", label: "Activity", icon: History },
    ],
  },
  {
    label: "You",
    items: [
      { to: "/profile", label: "Profile", icon: UserRound },
      { to: "/resumes", label: "Resumes", icon: FileText },
    ],
  },
];

const shortcutItems = navGroups.flatMap((group) => group.items);

const settingsItem: NavItem = { to: "/settings", label: "Settings", icon: SettingsIcon };

/// Layout is the app shell: the sidebar, and beside it the sections.
///
/// A section the user leaves is hidden rather than unmounted, as a tab in a
/// native app is: its search, its selection, a half-typed draft and every
/// scroll position inside it are still there when they come back, with no
/// code to save and restore them. React's <Activity> does the hiding — it
/// keeps the DOM and the state, and pauses the section's effects so a hidden
/// screen does not keep polling. Each section gets its own <Routes> with the
/// location it was left at, so a hidden Jobs goes on seeing its own
/// /jobs/123?q=go rather than the path the window is at now.
export function Layout() {
  const location = useLocation();
  const active = sectionFor(location.pathname);
  const visited = useVisitedSections(location, active);
  const hrefFor = (root: string) => sectionHref(root, active, visited);
  useNavigationShortcuts(hrefFor);
  usePreloadedSections();

  if (!active) return <Navigate to="/" replace />;

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background text-foreground">
      <Sidebar active={active} hrefFor={hrefFor} />
      <main className="relative min-w-0 flex-1 overflow-hidden">
        <RequireDaemon>
          <RequireSetup>
            {SECTIONS.map((section) => {
              const at = visited.get(section.root);
              if (!at) return null;
              return (
                <Activity key={section.root} mode={section === active ? "visible" : "hidden"}>
                  <SectionView section={section} at={at} />
                </Activity>
              );
            })}
          </RequireSetup>
        </RequireDaemon>
      </main>
    </div>
  );
}

/// SectionView is one section at the location it was left at. It scrolls on
/// its own, so leaving it leaves its position where it was, and it hosts the
/// popups opened inside it (see lib/portal-container.ts).
function SectionView({ section, at }: { section: Section; at: Location }) {
  const [portalHost, setPortalHost] = useState<HTMLElement | null>(null);
  return (
    <PortalContainer value={portalHost}>
      {/* A split view fills this box absolutely, sizing its panes to the
          window rather than to its content. */}
      <div className="absolute inset-0 overflow-y-auto">
        <ErrorBoundary resetKey={at.pathname}>
          {/* Nothing in its place while a screen's module loads: it comes
              from disk, and is preloaded, so a spinner would only flash. */}
          <Suspense fallback={null}>
            <Routes location={at}>{section.routes}</Routes>
          </Suspense>
        </ErrorBoundary>
      </div>
      <div ref={setPortalHost} />
    </PortalContainer>
  );
}

/// preloadDelay is how long after the shell appears the other screens are
/// loaded, so loading them does not compete with drawing the first.
const preloadDelay = 1000;

/// usePreloadedSections loads every section's screen once the shell is up,
/// so the first visit to one does not wait on it.
function usePreloadedSections() {
  useEffect(() => {
    const timer = setTimeout(preloadSections, preloadDelay);
    return () => clearTimeout(timer);
  }, []);
}

/// useVisitedSections is where each section was last left, by root. A
/// section appears in it once visited, and is rendered from then on.
function useVisitedSections(location: Location, active: Section | undefined) {
  const [visited, setVisited] = useState(() => new Map<string, Location>());
  // Recorded during render rather than in an effect, so the section being
  // navigated to is drawn at the new location in the same pass instead of
  // one frame late.
  if (active && visited.get(active.root) !== location) {
    const next = new Map(visited).set(active.root, location);
    setVisited(next);
    return next;
  }
  return visited;
}

/// sectionHref is where a section's sidebar link and shortcut go: back to
/// where it was left. From inside the section, it goes to the section's own
/// list with nothing selected, keeping the list's search and filters.
function sectionHref(root: string, active: Section | undefined, visited: Map<string, Location>) {
  if (active?.root === root) return { pathname: root, search: visited.get(root)?.search ?? "" };
  const at = visited.get(root);
  return at ? { pathname: at.pathname, search: at.search } : { pathname: root, search: "" };
}

type HrefFor = (root: string) => { pathname: string; search: string };

function Sidebar({ active, hrefFor }: { active: Section; hrefFor: HrefFor }) {
  return (
    <aside className="flex w-56 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
      {/* The top strip is where macOS draws the window controls, and on
          every platform it is somewhere to drag the window from. */}
      <div data-tauri-drag-region className="flex h-13 shrink-0 items-center px-4">
        {!isMac && (
          <span data-tauri-drag-region className="text-sm font-semibold">
            JobPilot
          </span>
        )}
      </div>

      <nav className="flex flex-1 flex-col gap-4 overflow-y-auto px-2.5 pb-3">
        {navGroups.map((group, i) => (
          <div key={group.label ?? i} className="flex flex-col gap-px">
            {group.label && (
              <p className="px-2.5 pb-1 text-[11px] font-semibold text-muted-foreground">{group.label}</p>
            )}
            {group.items.map((item) => (
              <SidebarLink key={item.to} item={item} active={active.root === item.to} to={hrefFor(item.to)} />
            ))}
          </div>
        ))}
      </nav>

      <div className="flex flex-col gap-2 px-2.5 pb-3">
        <WhenConnected>
          <SetupCallout />
        </WhenConnected>
        <WhenConnected>
          <WorkspaceSwitcher />
        </WhenConnected>
      </div>
    </aside>
  );
}

function SidebarLink({
  item,
  active,
  to,
}: {
  item: NavItem;
  active: boolean;
  to: { pathname: string; search: string };
}) {
  const Icon = item.icon;
  const index = shortcutItems.indexOf(item);
  const shortcut = item.to === "/settings" ? `${modLabel},` : index >= 0 ? `${modLabel}${index + 1}` : undefined;
  return (
    <Link
      to={to}
      aria-current={active ? "page" : undefined}
      title={shortcut ? `${item.label} (${shortcut})` : item.label}
      className={cn(
        "group flex h-7 items-center gap-2.5 rounded-md px-2.5 text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring",
        active
          ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
          : "text-subtle-foreground hover:bg-sidebar-accent/60",
      )}
    >
      <Icon className={cn("size-4 shrink-0", active ? "text-primary" : "text-muted-foreground")} strokeWidth={1.75} />
      <span className="flex-1 truncate">{item.label}</span>
      {item.to === "/applications" && (
        <WhenConnected>
          <WaitingCount />
        </WhenConnected>
      )}
    </Link>
  );
}

/// WhenConnected renders the sidebar's live parts only once the daemon is
/// reachable. The sidebar is drawn while the connection is still being made,
/// and anything that queries the daemon before then would throw.
function WhenConnected({ children }: { children: React.ReactNode }) {
  return useDaemon().status === "ready" ? children : null;
}

/// WaitingCount is the number of applications that JobPilot has finished
/// with and a person has not: ready for review, or approved and not yet
/// sent. It is the only badge in the sidebar, because it is the only count
/// that asks for something.
function WaitingCount() {
  const { candidateId } = useCandidateId();
  const { data } = useApplications(candidateId);
  const waiting = data?.filter((a) => a.state === "READY_FOR_REVIEW" || a.state === "APPROVED").length ?? 0;
  if (waiting === 0) return null;
  return (
    <span className="rounded-full bg-primary px-1.5 text-[11px] leading-4.5 font-semibold text-primary-foreground tabular-nums">
      {waiting}
    </span>
  );
}

/// useNavigationShortcuts binds ⌘1–⌘7 (Ctrl on Windows and Linux) to the
/// sidebar's screens in order, and ⌘, to Settings, as native apps do. Like
/// the sidebar, a shortcut returns to where the section was left.
function useNavigationShortcuts(hrefFor: HrefFor) {
  const navigate = useNavigate();
  // Read through a ref so the listener is bound once, not on every
  // navigation.
  const hrefForRef = useRef(hrefFor);
  useEffect(() => {
    hrefForRef.current = hrefFor;
  });
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const mod = isMac ? event.metaKey : event.ctrlKey;
      if (!mod || event.altKey || event.shiftKey) return;
      if (event.key === ",") {
        event.preventDefault();
        navigate(hrefForRef.current(settingsItem.to));
        return;
      }
      const index = Number(event.key) - 1;
      if (Number.isInteger(index) && index >= 0 && index < shortcutItems.length) {
        event.preventDefault();
        navigate(hrefForRef.current(shortcutItems[index].to));
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [navigate]);
}

/// RequireSetup sends a brand-new installation to the first-run wizard
/// instead of to a dashboard it cannot fill in. No candidate profile is what
/// "brand new" means here — it is the first thing onboarding creates and the
/// one every other screen depends on.
///
/// Skipping is honoured for the rest of the session, so the wizard guides
/// rather than traps: the screens' own empty states still say what is
/// missing for anyone who would rather look around first.
function RequireSetup({ children }: { children: React.ReactNode }) {
  const { activeWorkspace } = useDaemon();
  const { data: profile, dataUpdatedAt, errorUpdatedAt } = useProfile();

  // Deciding before the answer arrives would either flash the dashboard or
  // redirect someone who has a profile. Only the first answer is waited for:
  // a query that failed with no data reads as pending again while it
  // refetches, and unmounting the screens then remounts them, which
  // refetches again — a request loop for as long as the daemon is down.
  const answered = dataUpdatedAt > 0 || errorUpdatedAt > 0;
  if (!answered) return <p className="text-sm text-muted-foreground">Loading…</p>;
  if (profile === null && activeWorkspace && !setupSkipped(activeWorkspace.id))
    return <Navigate to="/welcome" replace />;

  return <>{children}</>;
}

/// SetupCallout is the way back to the wizard for an installation that is
/// past the first-run redirect but still cannot work: someone who skipped
/// setup, or who created a profile from the Profile screen and never went
/// near the rest. Only a missing profile redirects, on the grounds that
/// anyone further along is navigating deliberately and should not be
/// hijacked — but they still need to be able to find the unfinished steps.
/// It lives in the sidebar so it is on every screen without pushing any
/// screen's content down.
function SetupCallout() {
  const { data: llm } = useLLMSettings();
  const { data: searches } = useSavedSearches();
  if (!llm || !searches) return null;

  const missing = [
    searches.length === 0 && "a job search to discover jobs with",
    !llm.provider && "a local model to use",
  ].filter(Boolean);
  if (missing.length === 0) return null;

  return (
    <div data-slot="setup-banner" className="rounded-lg border border-warning/30 bg-warning/10 p-2.5">
      <p className="text-xs font-medium text-warning">Setup isn’t finished</p>
      <p className="mt-0.5 text-xs text-subtle-foreground">JobPilot still needs {missing.join(" and ")}.</p>
      <Button size="xs" variant="outline" className="mt-2" render={<Link to="/welcome">Finish setup</Link>} />
    </div>
  );
}
