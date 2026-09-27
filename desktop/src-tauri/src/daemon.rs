// Daemon lifecycle: locate a running jobpilot daemon (via the discovery
// file it writes) or start one, mirroring cmd/jobpilot-cli/daemon.go's
// logic exactly so the CLI and desktop app agree on how to find/start the
// same daemon. See docs/09_decisions/0012-loopback-http-ipc.md.

use serde::Deserialize;
use std::path::PathBuf;
use std::time::{Duration, Instant};

const DISCOVERY_FILE_NAME: &str = "daemon.json";
const DAEMON_START_TIMEOUT: Duration = Duration::from_secs(5);
// How long find_daemon waits for a daemon that went away to come back. It
// covers a restart, including one under `make dev` that waits for the old
// daemon to shut down before starting the new one.
const DAEMON_RETURN_TIMEOUT: Duration = Duration::from_secs(15);
const DAEMON_POLL_INTERVAL: Duration = Duration::from_millis(150);
const DAEMON_HEALTH_TIMEOUT: Duration = Duration::from_secs(2);

#[derive(Deserialize)]
struct DiscoveryInfo {
    #[allow(dead_code)]
    pid: u32,
    port: u16,
}

/// data_dir returns the application data directory, matching Go's
/// system.GetDataDir() (os.UserConfigDir() + "jobpilot") on all three
/// platforms.
fn data_dir() -> Result<PathBuf, String> {
    let config_dir = dirs::config_dir().ok_or("could not resolve the user config directory")?;
    Ok(config_dir.join("jobpilot"))
}

fn discovery_path() -> Result<PathBuf, String> {
    Ok(data_dir()?.join(DISCOVERY_FILE_NAME))
}

fn health_check(port: u16) -> bool {
    let url = format!("http://127.0.0.1:{port}/v1/health");
    ureq::get(&url)
        .config()
        .timeout_global(Some(DAEMON_HEALTH_TIMEOUT))
        .build()
        .call()
        .is_ok()
}

/// try_existing_daemon reads the discovery file left by a running daemon
/// and confirms it is actually reachable — a stale file can be left behind
/// by an unclean shutdown, so the file alone is never trusted.
fn try_existing_daemon() -> Option<u16> {
    let path = discovery_path().ok()?;
    let data = std::fs::read(path).ok()?;
    let info: DiscoveryInfo = serde_json::from_slice(&data).ok()?;
    if health_check(info.port) {
        Some(info.port)
    } else {
        None
    }
}

/// find_daemon_binary looks for the jobpilot daemon binary next to the
/// running desktop app first, then PATH, then (for local development, since
/// `make build` puts it in the repo's bin/ directory rather than next to
/// this Tauri binary's target/ output) a JOBPILOT_DAEMON_PATH override.
fn find_daemon_binary() -> Result<PathBuf, String> {
    let name = if cfg!(windows) {
        "jobpilot.exe"
    } else {
        "jobpilot"
    };

    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            let candidate = dir.join(name);
            if candidate.is_file() {
                return Ok(candidate);
            }
        }
    }

    if let Ok(path) = which(name) {
        return Ok(path);
    }

    if let Ok(override_path) = std::env::var("JOBPILOT_DAEMON_PATH") {
        return Ok(PathBuf::from(override_path));
    }

    Err(format!(
        "could not locate the {name:?} daemon binary next to the running executable, on PATH, \
         or via JOBPILOT_DAEMON_PATH"
    ))
}

fn which(name: &str) -> Result<PathBuf, ()> {
    let path_var = std::env::var_os("PATH").ok_or(())?;
    for dir in std::env::split_paths(&path_var) {
        let candidate = dir.join(name);
        if candidate.is_file() {
            return Ok(candidate);
        }
    }
    Err(())
}

/// spawn_daemon starts the jobpilot daemon binary as a detached background
/// process, with its output sent to the same daemon.log file the CLI uses.
fn spawn_daemon() -> Result<(), String> {
    let daemon_path = find_daemon_binary()?;
    let dir = data_dir()?;
    std::fs::create_dir_all(&dir).map_err(|e| format!("creating data directory: {e}"))?;

    let log_path = dir.join("daemon.log");
    let log_file = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(&log_path)
        .map_err(|e| format!("opening daemon log file: {e}"))?;
    let log_file_err = log_file
        .try_clone()
        .map_err(|e| format!("duplicating daemon log handle: {e}"))?;

    std::process::Command::new(daemon_path)
        .stdout(log_file)
        .stderr(log_file_err)
        .spawn()
        .map_err(|e| format!("launching daemon binary: {e}"))?;

    Ok(())
}

/// ensure_daemon connects to a running jobpilot daemon, starting one if
/// none is currently reachable, and returns the port it's listening on.
pub fn ensure_daemon() -> Result<u16, String> {
    if let Some(port) = try_existing_daemon() {
        return Ok(port);
    }

    spawn_daemon()?;

    let deadline = Instant::now() + DAEMON_START_TIMEOUT;
    while Instant::now() < deadline {
        if let Some(port) = try_existing_daemon() {
            return Ok(port);
        }
        std::thread::sleep(DAEMON_POLL_INTERVAL);
    }

    Err(format!(
        "jobpilot daemon did not become ready within {:?}",
        DAEMON_START_TIMEOUT
    ))
}

/// find_daemon waits for a running daemon and returns its port, and never
/// starts one. It is for a connection that was lost: the daemon that went
/// away may be restarting, and one started here in that gap would be a
/// second daemon on the same database. If none comes back, the app says so
/// and the user can start one with Retry (ensure_daemon).
pub fn find_daemon() -> Result<u16, String> {
    let deadline = Instant::now() + DAEMON_RETURN_TIMEOUT;
    loop {
        if let Some(port) = try_existing_daemon() {
            return Ok(port);
        }
        if Instant::now() >= deadline {
            return Err(format!(
                "the jobpilot daemon stopped and did not come back within {:?}",
                DAEMON_RETURN_TIMEOUT
            ));
        }
        std::thread::sleep(DAEMON_POLL_INTERVAL);
    }
}
