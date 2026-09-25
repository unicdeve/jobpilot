import { cn } from "cn";
import {
  Briefcase,
  FileText,
  History,
  House,
  Radar,
  Send,
  Settings as SettingsIcon,
  UserRound,
  type LucideIcon,
} from "lucide-react";
import { useEffect, useRef } from "react";
import { Link, Navigate, NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useDaemon } from "../lib/daemon";
import { isMac, modLabel } from "../lib/platform";
import {
  useApplications,
  useCandidateId,
  useLLMSettings,
  useProfile,
  useSavedSearches,
} from "../lib/queries";
import { useScrollRestoration } from "../lib/use-scroll-restoration";
import { setupSkipped } from "../pages/welcome";
import { ErrorBoundary } from "./error-boundary";
import { RequireDaemon } from "./require-daemon";
import { Button } from "./ui/button";

interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  end?: boolean;
}

/// Screens are grouped by what they are for: the work itself, what runs on
/// the user's behalf, and what JobPilot knows about them. The order is also
/// the order of the ⌘1–⌘7 shortcuts.
const navGroups: { label?: string; items: NavItem[] }[] = [
  {
    items: [
      { to: "/", label: "Dashboard", icon: House, end: true },
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

export function Layout() {
  const { pathname } = useLocation();
  // <main> is the app's only scrolling element — the window itself never
  // scrolls — so the shell that owns it is what remembers each screen's
  // position across navigation.
  const scrollRef = useRef<HTMLElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  useScrollRestoration(scrollRef, contentRef, pathname);
  useNavigationShortcuts();

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-background text-foreground">
      <Sidebar />
      {/* relative: a split view fills this region absolutely, so it can size
          its panes to the window rather than to its content. */}
      <main ref={scrollRef} className="relative min-w-0 flex-1 overflow-y-auto">
        <div ref={contentRef} className="min-h-full">
          <RequireDaemon>
            <RequireSetup>
              <RouteErrorBoundary>
                <Outlet />
              </RouteErrorBoundary>
            </RequireSetup>
          </RequireDaemon>
        </div>
      </main>
    </div>
  );
}

function Sidebar() {
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
              <SidebarLink key={item.to} item={item} />
            ))}
          </div>
        ))}
      </nav>

      <div className="flex flex-col gap-2 px-2.5 pb-3">
        <WhenConnected>
          <SetupCallout />
        </WhenConnected>
        <SidebarLink item={{ to: "/settings", label: "Settings", icon: SettingsIcon }} />
      </div>
    </aside>
  );
}

function SidebarLink({ item }: { item: NavItem }) {
  const Icon = item.icon;
  const index = shortcutItems.indexOf(item);
  const shortcut = item.to === "/settings" ? `${modLabel},` : index >= 0 ? `${modLabel}${index + 1}` : undefined;
  return (
    <NavLink
      to={item.to}
      end={item.end}
      title={shortcut ? `${item.label} (${shortcut})` : item.label}
      className={({ isActive }) =>
        cn(
          "group flex h-7 items-center gap-2.5 rounded-md px-2.5 text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring",
          isActive
            ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
            : "text-subtle-foreground hover:bg-sidebar-accent/60",
        )
      }
    >
      {({ isActive }) => (
        <>
          <Icon
            className={cn("size-4 shrink-0", isActive ? "text-primary" : "text-muted-foreground")}
            strokeWidth={1.75}
          />
          <span className="flex-1 truncate">{item.label}</span>
          {item.to === "/applications" && (
            <WhenConnected>
              <WaitingCount />
            </WhenConnected>
          )}
        </>
      )}
    </NavLink>
  );
}

/// WhenConnected renders the sidebar's live parts only once the daemon is
/// reachable. The sidebar is drawn while the connection is still being made,
/// and anything that queries the daemon before then would throw.
function WhenConnected({ children }: { children: React.ReactNode }) {
  return useDaemon().status === "ready" ? <>{children}</> : null;
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
/// sidebar's screens in order, and ⌘, to Settings, as native apps do.
function useNavigationShortcuts() {
  const navigate = useNavigate();
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const mod = isMac ? event.metaKey : event.ctrlKey;
      if (!mod || event.altKey || event.shiftKey) return;
      if (event.key === ",") {
        event.preventDefault();
        navigate("/settings");
        return;
      }
      const index = Number(event.key) - 1;
      if (Number.isInteger(index) && index >= 0 && index < shortcutItems.length) {
        event.preventDefault();
        navigate(shortcutItems[index].to);
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
  const { isPending, isError: noProfile } = useProfile();

  // Deciding before the answer arrives would either flash the dashboard or
  // redirect someone who has a profile.
  if (isPending) return <p className="text-sm text-muted-foreground">Loading…</p>;
  if (noProfile && !setupSkipped()) return <Navigate to="/welcome" replace />;

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

function RouteErrorBoundary({ children }: { children: React.ReactNode }) {
  const { pathname } = useLocation();
  return <ErrorBoundary key={pathname}>{children}</ErrorBoundary>;
}
