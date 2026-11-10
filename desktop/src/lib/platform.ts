import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { open } from "@tauri-apps/plugin-dialog";
import { openUrl } from "@tauri-apps/plugin-opener";

/// platform is the one module that knows whether the app runs in the
/// desktop's webview or in a web browser, served by the daemon
/// (ADR-0035 §5). Nothing else imports a Tauri package.
///
/// Tauri defines __TAURI_INTERNALS__ before the page's scripts run.
export const isDesktop = "__TAURI_INTERNALS__" in window;

/// isMac decides the platform conventions the shell follows: where the
/// window controls are, and whether shortcuts use ⌘ or Ctrl.
export const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

/// The key that modifies an app shortcut, as a label: "⌘" or "Ctrl+".
export const modLabel = isMac ? "⌘" : "Ctrl+";

/// overlaidTitleBar is whether the window's controls sit over the page:
/// the desktop on macOS draws its title bar as an overlay
/// (src-tauri/tauri.macos.conf.json). A browser tab has none.
export const overlaidTitleBar = isDesktop && isMac;

// Exposed to CSS so the sidebar can clear macOS's traffic lights.
document.documentElement.dataset.platform = overlaidTitleBar ? "macos" : "other";

/// connectDaemon is the address of the daemon's API.
///
/// The desktop starts the daemon, or replaces an older one, and is told
/// where it listens; onWaiting is called while an older daemon stops
/// (ADR-0029 §4). A browser was served the page by the daemon, so the API
/// is at the page's own origin.
export async function connectDaemon(onWaiting: () => void): Promise<string> {
  if (!isDesktop) return "";
  const unlisten = await listen("daemon-waiting", onWaiting);
  try {
    return daemonAddress(await invoke<number>("ensure_daemon"));
  } finally {
    unlisten();
  }
}

/// findDaemon is the address of a daemon that is running, after the one
/// in use stopped answering. It never starts one.
///
/// The desktop finds a daemon that restarted on another port. A page in a
/// browser cannot follow it there: it only learns whether its own daemon
/// came back.
export async function findDaemon(): Promise<string> {
  if (isDesktop) return daemonAddress(await invoke<number>("find_daemon"));
  const answered = await fetch("/v1/health").then(
    (res) => res.ok,
    () => false,
  );
  if (answered) return "";
  throw new Error("JobPilot’s daemon has stopped.");
}

function daemonAddress(port: number): string {
  return `http://127.0.0.1:${port}`;
}

/// setWindowTheme tells the desktop's native window which theme the page
/// is in, so the title bar and frame match it; null follows the OS. A
/// browser draws its own frame.
export function setWindowTheme(theme: "light" | "dark" | null) {
  if (!isDesktop) return;
  void import("@tauri-apps/api/window")
    .then(({ getCurrentWindow }) => getCurrentWindow().setTheme(theme))
    .catch(() => {
      // A window that cannot be themed still has a themed page.
    });
}

/// appVersion is this app's own version, or null in a browser, where the
/// page is the daemon's and has no version apart from it.
export async function appVersion(): Promise<string | null> {
  return isDesktop ? invoke<string>("app_version") : null;
}

/// openExternal opens an http(s) link in the user's own browser: from the
/// desktop, outside the app; from a browser, in a new tab that cannot reach
/// back into this one.
export async function openExternal(url: string): Promise<void> {
  if (isDesktop) return openUrl(url);
  window.open(url, "_blank", "noopener,noreferrer");
}

/// ChosenFile is a file the user picked: its path in the desktop, which the
/// daemon reads from disk, or its content in a browser, which never has the
/// path (ADR-0035 §4).
export type ChosenFile = { kind: "path"; path: string } | { kind: "upload"; file: File };

/// chooseFile asks for one file of the given extensions (without the dot),
/// and is null when the user cancels.
export async function chooseFile(title: string, extensions: string[]): Promise<ChosenFile | null> {
  if (isDesktop) {
    const path = await open({ multiple: false, title, filters: [{ name: "Documents", extensions }] });
    if (!path || Array.isArray(path)) return null;
    return { kind: "path", path };
  }

  const input = document.createElement("input");
  input.type = "file";
  input.accept = extensions.map((extension) => `.${extension}`).join(",");
  return new Promise((resolve) => {
    input.addEventListener("change", () => {
      const file = input.files?.[0];
      resolve(file ? { kind: "upload", file } : null);
    });
    input.addEventListener("cancel", () => resolve(null));
    input.click();
  });
}
