import { HashRouter, Route, Routes } from "react-router-dom";
import { Layout } from "./components/layout";
import { DaemonProvider } from "./lib/daemon";
import { ToastProvider } from "./lib/toast";
import { ApplicationDetail } from "./pages/application-detail";
import { Applications } from "./pages/applications";
import { Dashboard } from "./pages/dashboard";
import { Jobs } from "./pages/jobs";
import { Matches } from "./pages/matches";
import { Profile } from "./pages/profile";
import { Resumes } from "./pages/resumes";
import { Settings } from "./pages/settings";

function App() {
  return (
    <ToastProvider>
      <DaemonProvider>
        <HashRouter>
          <Routes>
            <Route element={<Layout />}>
              <Route index element={<Dashboard />} />
              <Route path="profile" element={<Profile />} />
              <Route path="resumes" element={<Resumes />} />
              <Route path="jobs" element={<Jobs />} />
              <Route path="matches" element={<Matches />} />
              <Route path="applications" element={<Applications />} />
              <Route path="applications/:id" element={<ApplicationDetail />} />
              <Route path="settings" element={<Settings />} />
            </Route>
          </Routes>
        </HashRouter>
      </DaemonProvider>
    </ToastProvider>
  );
}

export default App;
