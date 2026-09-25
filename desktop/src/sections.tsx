import type { ReactNode } from "react";
import { Route } from "react-router-dom";
import { Activity, NoRunSelected } from "./pages/activity";
import { ActivityRun } from "./pages/activity-run";
import { ApplicationDetail } from "./pages/application-detail";
import { Applications, NoApplicationSelected } from "./pages/applications";
import { Dashboard } from "./pages/dashboard";
import { JobDetail } from "./pages/job-detail";
import { Jobs, NoJobSelected } from "./pages/jobs";
import { Profile } from "./pages/profile";
import { Resumes } from "./pages/resumes";
import { Scouts } from "./pages/scouts";
import { Settings } from "./pages/settings";

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
  const root = "/" + (pathname.split("/")[1] ?? "");
  return SECTIONS.find((section) => section.root === root);
}
