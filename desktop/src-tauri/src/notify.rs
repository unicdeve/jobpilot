// Delivering notifications: the mode the daemon starts the app in to post
// what a scout found, under JobPilot's own name, with no window
// (docs/09_decisions/0028-notifications.md §1).

use serde::Deserialize;
use std::time::Duration;
use tauri::AppHandle;
use tauri_plugin_notification::NotificationExt;

use crate::daemon;

/// DELIVER_FLAG is what the daemon starts the app with to deliver. It
/// matches desktopapp.DeliverFlag in the daemon.
pub const DELIVER_FLAG: &str = "--deliver-notifications";

const REQUEST_TIMEOUT: Duration = Duration::from_secs(5);

#[derive(Deserialize)]
struct Notification {
    id: String,
    title: String,
    body: String,
}

#[derive(Deserialize)]
struct Notifications {
    notifications: Vec<Notification>,
}

#[derive(Deserialize)]
struct Claim {
    claimed: bool,
}

/// delivering reports whether the app was started to deliver notifications.
pub fn delivering() -> bool {
    std::env::args().any(|arg| arg == DELIVER_FLAG)
}

/// deliver posts every notification the daemon says is waiting, and
/// returns how many it posted. Each is claimed first, and posted only if
/// the claim is this process's, so an open app and a delivery started at
/// the same moment never post one twice.
pub fn deliver(app: &AppHandle) -> Result<usize, String> {
    let port = daemon::existing_daemon().ok_or("no jobpilot daemon is running")?;
    let base = format!("http://127.0.0.1:{port}");

    let waiting: Notifications = ureq::get(&format!("{base}/v1/notifications/postable"))
        .config()
        .timeout_global(Some(REQUEST_TIMEOUT))
        .build()
        .call()
        .map_err(|err| format!("asking the daemon what to post: {err}"))?
        .body_mut()
        .read_json()
        .map_err(|err| format!("reading what to post: {err}"))?;

    let mut posted = 0;
    for notification in waiting.notifications {
        let claim_url = format!("{base}/v1/notifications/{}/posted", notification.id);
        let claim: Claim = ureq::post(&claim_url)
            .config()
            .timeout_global(Some(REQUEST_TIMEOUT))
            .build()
            .send_empty()
            .map_err(|err| format!("claiming a notification: {err}"))?
            .body_mut()
            .read_json()
            .map_err(|err| format!("reading a claim: {err}"))?;
        if !claim.claimed {
            continue;
        }
        app.notification()
            .builder()
            .title(&notification.title)
            .body(&notification.body)
            .show()
            .map_err(|err| format!("posting a notification: {err}"))?;
        posted += 1;
    }
    Ok(posted)
}
