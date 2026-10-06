import type { ReactNode } from "react";
import { Route } from "react-router-dom";
import { lazyPage } from "./lib/lazy-page";

// Every screen's module, loaded when the screen first renders. The shell
// preloads them all once it is up (preloadSections), so moving between
// sections does not wait on one.
const pages = {
  activity: () => import("./pages/activity"),
  activityRun: () => import("./pages/activity-run"),
  applicationDetail: () => import("./pages/application-detail"),
  applications: () => import("./pages/applications"),
  dashboard: () => import("./pages/dashboard"),
  jobDetail: () => import("./pages/job-detail"),
  jobs: () => import("./pages/jobs"),
  profile: () => import("./pages/profile"),
  resumes: () => import("./pages/resumes"),
  scouts: () => import("./pages/scouts"),
  settings: () => import("./pages/settings"),
};

const Activity = lazyPage(pages.activity, "Activity");
const NoRunSelected = lazyPage(pages.activity, "NoRunSelected");
const ActivityRun = lazyPage(pages.activityRun, "ActivityRun");
const ApplicationDetail = lazyPage(pages.applicationDetail, "ApplicationDetail");
const Applications = lazyPage(pages.applications, "Applications");
const NoApplicationSelected = lazyPage(pages.applications, "NoApplicationSelected");
const Dashboard = lazyPage(pages.dashboard, "Dashboard");
const JobDetail = lazyPage(pages.jobDetail, "JobDetail");
const Jobs = lazyPage(pages.jobs, "Jobs");
const NoJobSelected = lazyPage(pages.jobs, "NoJobSelected");
const Profile = lazyPage(pages.profile, "Profile");
const Resumes = lazyPage(pages.resumes, "Resumes");
const Scouts = lazyPage(pages.scouts, "Scouts");
const Settings = lazyPage(pages.settings, "Settings");

/// preloadSections loads every screen's module, for the shell to call once
/// the first screen is showing. A module that fails to load here is tried
/// again when its screen renders, which is where the failure is shown.
export function preloadSections() {
  for (const load of Object.values(pages)) void load().catch(() => {});
}

/// A Section is one of the sidebar's screens together with the routes inside
/// it. The app shell keeps a section alive while another is showing, so each
/// one is a tab rather than a page: see components/layout.tsx.
export interface Section {
  /// The first segment of every path in the section, and where its sidebar
  /// link goes before it has been visited.
  root: string;
  routes: ReactNode;
}

export const SECTIONS: Section[] = [
  { root: "/", routes: <Route index element={<Dashboard />} /> },
  {
    root: "/jobs",
    routes: (
      <Route path="jobs" element={<Jobs />}>
        <Route index element={<NoJobSelected />} />
        <Route path=":id" element={<JobDetail />} />
      </Route>
    ),
  },
  {
    root: "/applications",
    routes: (
      <Route path="applications" element={<Applications />}>
        <Route index element={<NoApplicationSelected />} />
        <Route path=":id" element={<ApplicationDetail />} />
      </Route>
    ),
  },
  { root: "/scouts", routes: <Route path="scouts" element={<Scouts />} /> },
  {
    root: "/activity",
    routes: (
      <Route path="activity" element={<Activity />}>
        <Route index element={<NoRunSelected />} />
        <Route path=":id" element={<ActivityRun />} />
      </Route>
    ),
  },
  { root: "/profile", routes: <Route path="profile" element={<Profile />} /> },
  { root: "/resumes", routes: <Route path="resumes" element={<Resumes />} /> },
  { root: "/settings", routes: <Route path="settings" element={<Settings />} /> },
];

/// sectionFor is the section a path belongs to, by its first segment, or
/// undefined for a path no section holds.
export function sectionFor(pathname: string): Section | undefined {
  const root = `/${pathname.split("/")[1] ?? ""}`;
  return SECTIONS.find((section) => section.root === root);
}
