import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useDaemon } from "../lib/daemon";
import { ErrorBoundary } from "./error-boundary";
import { Button } from "./ui/button";

const navItems = [
  { to: "/", label: "Dashboard", end: true },
  { to: "/profile", label: "Profile" },
  { to: "/resumes", label: "Resumes" },
  { to: "/jobs", label: "Jobs" },
  { to: "/matches", label: "Matches" },
  { to: "/applications", label: "Applications" },
  { to: "/settings", label: "Settings" },
];

export function Layout() {
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
      <main className="flex-1 overflow-y-auto p-6">
        <RequireDaemon>
          <RouteErrorBoundary>
            <Outlet />
          </RouteErrorBoundary>
        </RequireDaemon>
      </main>
    </div>
  );
}

function RequireDaemon({ children }: { children: React.ReactNode }) {
  const { status, error, retry } = useDaemon();

  if (status === "connecting") {
    return <p className="text-sm text-slate-500">Connecting to the jobpilot daemon…</p>;
  }

  if (status === "error") {
    return (
      <div className="max-w-md space-y-3 rounded-md border border-red-200 bg-red-50 p-4">
        <p className="text-sm font-medium text-red-800">Could not connect to the jobpilot daemon.</p>
        <p className="text-sm text-red-700">{error}</p>
        <Button variant="secondary" onClick={retry}>
          Retry
        </Button>
      </div>
    );
  }

  return <>{children}</>;
}

function RouteErrorBoundary({ children }: { children: React.ReactNode }) {
  const { pathname } = useLocation();
  return <ErrorBoundary key={pathname}>{children}</ErrorBoundary>;
}
