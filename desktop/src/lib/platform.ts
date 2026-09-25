/// isMac decides the platform conventions the shell follows: where the
/// window controls are, and whether shortcuts use ⌘ or Ctrl.
export const isMac = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

/// The key that modifies an app shortcut, as a label: "⌘" or "Ctrl+".
export const modLabel = isMac ? "⌘" : "Ctrl+";

// Exposed to CSS so the sidebar can clear macOS's traffic lights, which sit
// over the window's content because the title bar is drawn as an overlay
// (src-tauri/tauri.macos.conf.json).
document.documentElement.dataset.platform = isMac ? "macos" : "other";
