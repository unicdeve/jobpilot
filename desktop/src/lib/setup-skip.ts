// Whether the user dismissed first-run setup. Kept apart from the wizard
// (pages/welcome.tsx) because the app shell reads it on every launch, and
// the wizard is loaded only when it is shown.

/// skipKey records that the user dismissed first-run setup. It lives in
/// sessionStorage, not the database: skipping is a decision about this
/// sitting, and an install that is still unfinished should offer to help
/// again next time it is opened.
const skipKey = "jobpilot.setup-skipped";

function key(workspaceID: string) {
  return `${skipKey}.${workspaceID}`;
}

/// setupSkipped reports whether the user dismissed the wizard. Reading
/// storage can throw outright (a webview with site data blocked), and a
/// failure here must not take the app down with it — treating that as
/// "not skipped" shows a wizard nobody asked for at worst.
export function setupSkipped(workspaceID: string): boolean {
  try {
    return sessionStorage.getItem(key(workspaceID)) === "1";
  } catch {
    return false;
  }
}

export function rememberSkip(workspaceID: string) {
  try {
    sessionStorage.setItem(key(workspaceID), "1");
  } catch {
    // Nothing to do: the wizard offers itself again, which is the safe way
    // to be wrong.
  }
}
