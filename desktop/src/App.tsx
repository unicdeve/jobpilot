import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { HashRouter, Route, Routes } from "react-router-dom";
import { Layout } from "./components/layout";
import { RequireDaemon } from "./components/require-daemon";
import { DaemonProvider } from "./lib/daemon";
import { ToastProvider } from "./lib/toast";
import { ApplicationDetail } from "./pages/application-detail";
import { Applications } from "./pages/applications";
import { Dashboard } from "./pages/dashboard";
import { JobDetail } from "./pages/job-detail";
import { Jobs } from "./pages/jobs";
import { Matches } from "./pages/matches";
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
                  <div className="p-6">
                    <RequireDaemon>
                      <Welcome />
                    </RequireDaemon>
                  </div>
                }
              />
              <Route element={<Layout />}>
                <Route index element={<Dashboard />} />
                <Route path="profile" element={<Profile />} />
                <Route path="resumes" element={<Resumes />} />
                <Route path="jobs" element={<Jobs />} />
                <Route path="jobs/:id" element={<JobDetail />} />
                <Route path="matches" element={<Matches />} />
                <Route path="applications" element={<Applications />} />
                <Route path="applications/:id" element={<ApplicationDetail />} />
                <Route path="settings" element={<Settings />} />
              </Route>
            </Routes>
          </HashRouter>
        </DaemonProvider>
      </ToastProvider>
    </QueryClientProvider>
  );
}

export default App;
