mod daemon;

// Both block for seconds at a time, so they run off the main thread: a
// command without `async` runs on it, and would freeze the window while it
// waits.
#[tauri::command]
async fn ensure_daemon() -> Result<u16, String> {
    tauri::async_runtime::spawn_blocking(daemon::ensure_daemon)
        .await
        .map_err(|e| e.to_string())?
}

#[tauri::command]
async fn find_daemon() -> Result<u16, String> {
    tauri::async_runtime::spawn_blocking(daemon::find_daemon)
        .await
        .map_err(|e| e.to_string())?
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .invoke_handler(tauri::generate_handler![ensure_daemon, find_daemon])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
