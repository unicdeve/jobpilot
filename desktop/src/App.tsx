import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { HashRouter, Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "./components/layout";
import { RequireDaemon } from "./components/require-daemon";
import { DaemonProvider } from "./lib/daemon";
import "./lib/platform";
import { ThemeProvider } from "./lib/theme";
import { ToastProvider } from "./lib/toast";
import { Activity, NoRunSelected } from "./pages/activity";
import { ActivityRun } from "./pages/activity-run";
import { ApplicationDetail } from "./pages/application-detail";
import { Applications, NoApplicationSelected } from "./pages/applications";
import { Scouts } from "./pages/scouts";
import { Dashboard } from "./pages/dashboard";
import { JobDetail } from "./pages/job-detail";
import { Jobs, NoJobSelected } from "./pages/jobs";
import { Profile } from "./pages/profile";
import { Resumes } from "./pages/resumes";
import { Settings } from "./pages/settings";
import { Welcome } from "./pages/welcome";

// The daemon is a local process reached over loopback, so refetching is
// cheap — but it is also a desktop app, not a browser tab left open for
// days, so window focus is not a meaningful signal that data went stale.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
      <ToastProvider>
        <DaemonProvider>
          <HashRouter>
            <Routes>
              {/* Outside <Layout>: first-run setup has nothing to navigate
                  to yet, so it gets the whole window rather than a sidebar
                  of screens that cannot do anything. */}
              <Route
                path="welcome"
                element={
                  <div className="h-screen overflow-y-auto">
                    <div data-tauri-drag-region className="h-13" />
                    <div className="px-8 pb-10">
                      <RequireDaemon>
                        <Welcome />
                      </RequireDaemon>
                    </div>
                  </div>
                }
              />
              <Route element={<Layout />}>
                <Route index element={<Dashboard />} />
                <Route path="jobs" element={<Jobs />}>
                  <Route index element={<NoJobSelected />} />
                  <Route path=":id" element={<JobDetail />} />
                </Route>
                {/* Matches were folded into Jobs: a match is a job with a
                    score, ranked under Jobs → Analyzed. */}
                <Route path="matches" element={<Navigate to="/jobs?view=analyzed" replace />} />
                <Route path="applications" element={<Applications />}>
                  <Route index element={<NoApplicationSelected />} />
                  <Route path=":id" element={<ApplicationDetail />} />
                </Route>
                <Route path="scouts" element={<Scouts />} />
                <Route path="activity" element={<Activity />}>
                  <Route index element={<NoRunSelected />} />
                  <Route path=":id" element={<ActivityRun />} />
                </Route>
                <Route path="profile" element={<Profile />} />
                <Route path="resumes" element={<Resumes />} />
                <Route path="settings" element={<Settings />} />
              </Route>
            </Routes>
          </HashRouter>
        </DaemonProvider>
      </ToastProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

export default App;
