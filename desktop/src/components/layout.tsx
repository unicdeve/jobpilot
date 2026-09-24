import { useRef } from "react";
import { Link, Navigate, NavLink, Outlet, useLocation } from "react-router-dom";
import { useLLMSettings, useProfile, useSavedSearches } from "../lib/queries";
import { setupSkipped } from "../pages/welcome";
import { useScrollRestoration } from "../lib/use-scroll-restoration";
import { ErrorBoundary } from "./error-boundary";
import { RequireDaemon } from "./require-daemon";
import { Button } from "./ui/button";

const navItems = [
  { to: "/", label: "Dashboard", end: true },
  { to: "/profile", label: "Profile" },
  { to: "/resumes", label: "Resumes" },
  { to: "/jobs", label: "Jobs" },
  { to: "/matches", label: "Matches" },
  { to: "/scouts", label: "Scouts" },
  { to: "/applications", label: "Applications" },
  { to: "/activity", label: "Activity" },
  { to: "/settings", label: "Settings" },
];

export function Layout() {
  const { pathname } = useLocation();
  // <main> is the app's only scrolling element — the window itself never
  // scrolls — so the shell that owns it is what remembers each screen's
  // position across navigation.
  const scrollRef = useRef<HTMLElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  useScrollRestoration(scrollRef, contentRef, pathname);

  return (
    <div className="flex h-screen w-screen bg-slate-50 text-slate-900">
      <aside className="flex w-52 shrink-0 flex-col border-r border-slate-200 bg-white">
        <div className="px-4 py-4 text-lg font-semibold">JobPilot</div>
        <nav className="flex flex-col gap-0.5 px-2">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `rounded-md px-3 py-1.5 text-sm font-medium ${
                  isActive ? "bg-slate-900 text-white" : "text-slate-600 hover:bg-slate-100"
                }`
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <main ref={scrollRef} className="flex-1 overflow-y-auto p-6">
        <div ref={contentRef}>
          <RequireDaemon>
            <RequireSetup>
              <SetupBanner />
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
  if (isPending) return <p className="text-sm text-slate-500">Loading…</p>;
  if (noProfile && !setupSkipped()) return <Navigate to="/welcome" replace />;

  return <>{children}</>;
}

/// SetupBanner is the way back to the wizard for an installation that is
/// past the first-run redirect but still cannot work: someone who skipped
/// setup, or who created a profile from the Profile screen and never went
/// near the rest. Only a missing profile redirects, on the grounds that
/// anyone further along is navigating deliberately and should not be
/// hijacked — but they still need to be able to find the unfinished steps.
function SetupBanner() {
  const { data: llm } = useLLMSettings();
  const { data: searches } = useSavedSearches();
  if (!llm || !searches) return null;

  const missing = [
    searches.length === 0 && "save a search before it can discover jobs",
    !llm.provider && "know which local LLM to use",
  ].filter(Boolean);
  if (missing.length === 0) return null;

  return (
    <div
      data-slot="setup-banner"
      className="mb-4 flex items-center justify-between gap-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2"
    >
      <p className="text-sm text-amber-800">JobPilot needs to {missing.join(", and to ")}.</p>
      <Button variant="secondary" render={<Link to="/welcome">Finish setup</Link>} />
    </div>
  );
}

function RouteErrorBoundary({ children }: { children: React.ReactNode }) {
  const { pathname } = useLocation();
  return <ErrorBoundary key={pathname}>{children}</ErrorBoundary>;
}
