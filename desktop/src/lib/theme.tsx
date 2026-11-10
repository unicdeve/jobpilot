import { createContext, type ReactNode, useContext, useEffect, useState } from "react";
import { setWindowTheme } from "./platform";

/// ThemePreference is what the user chose. "system" follows the operating
/// system, and keeps following it when it changes.
export type ThemePreference = "system" | "light" | "dark";

const STORAGE_KEY = "jobpilot.theme";

/// readPreference is wrapped because storage can be unavailable (a locked
/// down webview, a private window in a browser during development), and a
/// missing preference is not an error: it means "system".
function readPreference(): ThemePreference {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "light" || stored === "dark" || stored === "system") return stored;
  } catch {
    // fall through
  }
  return "system";
}

function writePreference(preference: ThemePreference) {
  try {
    localStorage.setItem(STORAGE_KEY, preference);
  } catch {
    // The choice still applies for this session.
  }
}

const darkQuery = () => window.matchMedia("(prefers-color-scheme: dark)");

function resolve(preference: ThemePreference): "light" | "dark" {
  if (preference === "system") return darkQuery().matches ? "dark" : "light";
  return preference;
}

/// applyTheme sets the class every colour token keys off, and tells the
/// native window, so the title bar and traffic lights on macOS (and the
/// frame elsewhere) match the page rather than the OS default.
function applyTheme(preference: ThemePreference) {
  const resolved = resolve(preference);
  document.documentElement.classList.toggle("dark", resolved === "dark");

  // null hands the choice back to the OS, which is what "system" means.
  setWindowTheme(preference === "system" ? null : preference);
}

// Applied before React renders, so the first frame is already in the right
// theme rather than flashing light on a dark desktop.
applyTheme(readPreference());

interface ThemeState {
  preference: ThemePreference;
  resolved: "light" | "dark";
  setPreference: (preference: ThemePreference) => void;
}

const ThemeContext = createContext<ThemeState | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(readPreference);
  const [resolved, setResolved] = useState<"light" | "dark">(() => resolve(preference));

  useEffect(() => {
    applyTheme(preference);
    setResolved(resolve(preference));
    if (preference !== "system") return;

    const query = darkQuery();
    const onChange = () => {
      applyTheme("system");
      setResolved(resolve("system"));
    };
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, [preference]);

  const setPreference = (next: ThemePreference) => {
    writePreference(next);
    setPreferenceState(next);
  };

  return <ThemeContext.Provider value={{ preference, resolved, setPreference }}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeState {
  const state = useContext(ThemeContext);
  if (!state) throw new Error("useTheme must be used inside ThemeProvider");
  return state;
}
