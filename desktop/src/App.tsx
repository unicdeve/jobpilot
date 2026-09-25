import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { HashRouter, Navigate, Route, Routes } from "react-router-dom";
import { Layout } from "./components/layout";
import { RequireDaemon } from "./components/require-daemon";
import { DaemonProvider } from "./lib/daemon";
import "./lib/platform";
import { ThemeProvider } from "./lib/theme";
import { ToastProvider } from "./lib/toast";
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
      // A section the user has left stays rendered but hidden, and a hidden
      // section's queries have no observers. Collecting them after the
      // default five minutes would empty a list the user expects to come
      // back to as they left it, scroll position and all.
      gcTime: 60 * 60_000,
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
              {/* Matches were folded into Jobs: a match is a job with a
                  score, ranked under Jobs → Analyzed. */}
              <Route path="matches" element={<Navigate to="/jobs?view=analyzed" replace />} />
              {/* Every other path is a section of the app shell, which does
                  its own routing so that sections stay alive between visits
                  (see components/layout.tsx and sections.tsx). */}
              <Route path="*" element={<Layout />} />
            </Routes>
          </HashRouter>
        </DaemonProvider>
      </ToastProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

export default App;
