mod daemon;
mod notify;
mod version;

use std::time::Duration;
use tauri::Emitter;

// Both block for seconds at a time, so they run off the main thread: a
// command without `async` runs on it, and would freeze the window while it
// waits.
//
// Replacing an older daemon waits for it to stop, so the window is told,
// with "daemon-waiting", what it is waiting on.
#[tauri::command]
async fn ensure_daemon(app: tauri::AppHandle) -> Result<u16, String> {
    tauri::async_runtime::spawn_blocking(move || {
        daemon::ensure_daemon(|| {
            let _ = app.emit("daemon-waiting", ());
        })
    })
    .await
    .map_err(|e| e.to_string())?
}

/// app_version is this app's version, shown under Settings → About beside
/// the daemon's (ADR-0029 §2).
#[tauri::command]
fn app_version() -> &'static str {
    version::app_version()
}

#[tauri::command]
async fn find_daemon() -> Result<u16, String> {
    tauri::async_runtime::spawn_blocking(daemon::find_daemon)
        .await
        .map_err(|e| e.to_string())?
}

/// POSTED_GRACE is how long a delivery waits after posting before it
/// exits, so the system has taken the notification from it.
const POSTED_GRACE: Duration = Duration::from_millis(500);

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let delivering = notify::delivering();
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_notification::init())
        .invoke_handler(tauri::generate_handler![
            ensure_daemon,
            find_daemon,
            app_version
        ])
        .setup(move |app| {
            if delivering {
                // Started by the daemon to post notifications: no window,
                // and on macOS no Dock icon either (ADR-0028 §1).
                #[cfg(target_os = "macos")]
                app.set_activation_policy(tauri::ActivationPolicy::Accessory);
                let handle = app.handle().clone();
                std::thread::spawn(move || {
                    if let Err(err) = notify::deliver(&handle) {
                        eprintln!("jobpilot: {err}");
                    }
                    std::thread::sleep(POSTED_GRACE);
                    handle.exit(0);
                });
                return Ok(());
            }

            // The main window is created here rather than by the
            // configuration, which would open it in delivery mode too.
            let config = app
                .config()
                .app
                .windows
                .iter()
                .find(|window| window.label == "main")
                .ok_or("the main window is missing from the configuration")?
                .clone();
            tauri::WebviewWindowBuilder::from_config(app.handle(), &config)?.build()?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
