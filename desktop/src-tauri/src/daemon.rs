// Daemon lifecycle: locate a running jobpilot daemon (via the discovery
// file it writes) or start one, mirroring
// internal/adapters/primary/httpapi/ensure.go's logic exactly so the CLI
// and desktop app agree on how to find, start and replace the same daemon.
// See docs/09_decisions/0012-loopback-http-ipc.md and ADR-0029 §4.

use crate::version;
use serde::Deserialize;
use std::cmp::Ordering;
use std::path::PathBuf;
use std::time::{Duration, Instant};

const DISCOVERY_FILE_NAME: &str = "daemon.json";
// The file a running daemon holds locked for as long as its process lives
// (internal/adapters/secondary/system/daemonlock.go).
const DAEMON_LOCK_NAME: &str = "daemon.lock";
const DAEMON_START_TIMEOUT: Duration = Duration::from_secs(5);
// How long to wait for a daemon to stop, or for one starting or stopping
// to settle. Stopping waits for a Scout run to record how it ended, which
// is seconds as a rule; this is the limit past which something is wrong.
const DAEMON_STOP_TIMEOUT: Duration = Duration::from_secs(120);
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

#[derive(Deserialize)]
struct Health {
    // Absent from a daemon built before versions were reported, which
    // compares as a development build.
    #[serde(default)]
    version: String,
}

/// health_check returns the daemon's version if it answers.
fn health_check(port: u16) -> Option<String> {
    let url = format!("http://127.0.0.1:{port}/v1/health");
    let mut response = ureq::get(&url)
        .config()
        .timeout_global(Some(DAEMON_HEALTH_TIMEOUT))
        .build()
        .call()
        .ok()?;
    let health: Health = response.body_mut().read_json().ok()?;
    Some(health.version)
}

/// daemon_running reports whether a daemon's process holds the lock: one
/// running, starting, or still stopping.
fn daemon_running() -> Result<bool, String> {
    let path = data_dir()?.join(DAEMON_LOCK_NAME);
    let file = match std::fs::OpenOptions::new()
        .read(true)
        .write(true)
        .create(true)
        .truncate(false)
        .open(&path)
    {
        Ok(file) => file,
        // No daemon has run from this data directory.
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => return Ok(false),
        Err(err) => return Err(format!("checking for a running jobpilot daemon: {err}")),
    };
    match file.try_lock() {
        Ok(()) => Ok(false),
        Err(std::fs::TryLockError::WouldBlock) => Ok(true),
        Err(std::fs::TryLockError::Error(err)) => {
            Err(format!("checking for a running jobpilot daemon: {err}"))
        }
    }
}

/// shutdown asks the daemon to stop. It answers before it has stopped.
fn shutdown(port: u16) -> Result<(), String> {
    let url = format!("http://127.0.0.1:{port}/v1/shutdown");
    ureq::post(&url)
        .config()
        .timeout_global(Some(DAEMON_HEALTH_TIMEOUT))
        .build()
        .send_empty()
        .map(|_| ())
        .map_err(|e| format!("asking the jobpilot daemon to stop: {e}"))
}

/// wait_for_exit waits until no daemon's process holds the lock.
fn wait_for_exit() -> Result<(), String> {
    let deadline = Instant::now() + DAEMON_STOP_TIMEOUT;
    while daemon_running()? {
        if Instant::now() >= deadline {
            return Err(format!(
                "the jobpilot daemon did not stop within {DAEMON_STOP_TIMEOUT:?}"
            ));
        }
        std::thread::sleep(DAEMON_POLL_INTERVAL);
    }
    Ok(())
}

/// settled_daemon returns the port and version of the daemon that answers,
/// or None when none is running. A daemon that holds the lock without
/// answering is starting or stopping, and is waited for: starting another
/// then would put two on one database.
fn settled_daemon() -> Result<Option<(u16, String)>, String> {
    let deadline = Instant::now() + DAEMON_STOP_TIMEOUT;
    loop {
        if let Some(found) = try_existing_daemon() {
            return Ok(Some(found));
        }
        if !daemon_running()? {
            return Ok(None);
        }
        if Instant::now() >= deadline {
            return Err(format!(
                "a jobpilot daemon is running but did not answer within {DAEMON_STOP_TIMEOUT:?}"
            ));
        }
        std::thread::sleep(DAEMON_POLL_INTERVAL);
    }
}

/// existing_daemon returns the port of a running daemon, and never starts
/// one. Delivering notifications uses it: the daemon that asked is running.
pub fn existing_daemon() -> Option<u16> {
    try_existing_daemon().map(|(port, _)| port)
}

/// register_desktop tells the daemon where this app's executable is, so it
/// can start the app to post a notification when the app is not open
/// (ADR-0028 §1). It says so on every start, so a moved or rebuilt app is
/// found. A failure costs only notifications posted while the app is
/// closed, so it is reported and not returned.
fn register_desktop(port: u16) {
    let executable = match std::env::current_exe().and_then(|path| path.canonicalize()) {
        Ok(path) => path,
        Err(err) => {
            eprintln!("could not tell the daemon where the app is: {err}");
            return;
        }
    };
    let url = format!("http://127.0.0.1:{port}/v1/desktop");
    let result = ureq::put(&url)
        .config()
        .timeout_global(Some(DAEMON_HEALTH_TIMEOUT))
        .build()
        .send_json(serde_json::json!({ "executable": executable }));
    if let Err(err) = result {
        eprintln!("could not tell the daemon where the app is: {err}");
    }
}

/// try_existing_daemon reads the discovery file left by a running daemon
/// and confirms it is actually reachable — a stale file can be left behind
/// by an unclean shutdown, so the file alone is never trusted. It returns
/// the daemon's port and version.
fn try_existing_daemon() -> Option<(u16, String)> {
    let path = discovery_path().ok()?;
    let data = std::fs::read(path).ok()?;
    let info: DiscoveryInfo = serde_json::from_slice(&data).ok()?;
    health_check(info.port).map(|version| (info.port, version))
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
///
/// Detached as the Go clients detach it, so it outlives the app (ADR-0027):
/// in a process group of its own, so a terminal's Ctrl+C under `tauri dev`
/// does not reach it, and on Windows with no console, which a release
/// build, having none of its own, would otherwise open a window for.
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

    let mut command = std::process::Command::new(daemon_path);
    command.stdout(log_file).stderr(log_file_err);
    detach(&mut command);
    command
        .spawn()
        .map_err(|e| format!("launching daemon binary: {e}"))?;

    Ok(())
}

#[cfg(unix)]
fn detach(command: &mut std::process::Command) {
    use std::os::unix::process::CommandExt;
    command.process_group(0);
}

#[cfg(windows)]
fn detach(command: &mut std::process::Command) {
    use std::os::windows::process::CommandExt;
    // A process group of its own, and no console.
    const CREATE_NEW_PROCESS_GROUP: u32 = 0x0000_0200;
    const DETACHED_PROCESS: u32 = 0x0000_0008;
    command.creation_flags(CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS);
}

/// ensure_daemon connects to a running jobpilot daemon, starting one if
/// none is currently reachable, and returns the port it's listening on.
///
/// A daemon older than this app is stopped and replaced, and one newer is
/// used (ADR-0029 §4). waiting is told when the app waits for the older
/// daemon to stop.
pub fn ensure_daemon(waiting: impl Fn()) -> Result<u16, String> {
    let port = find_or_start_daemon(waiting)?;
    register_desktop(port);
    Ok(port)
}

fn find_or_start_daemon(waiting: impl Fn()) -> Result<u16, String> {
    if let Some((port, daemon_version)) = settled_daemon()? {
        if version::compare(version::app_version(), &daemon_version) != Ordering::Greater {
            return Ok(port);
        }
        waiting();
        shutdown(port)?;
        wait_for_exit()?;
    }

    spawn_daemon()?;

    let deadline = Instant::now() + DAEMON_START_TIMEOUT;
    while Instant::now() < deadline {
        if let Some((port, _)) = try_existing_daemon() {
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
        if let Some((port, _)) = try_existing_daemon() {
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
