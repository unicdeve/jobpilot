import { readFileSync } from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import type { Plugin } from "vite";

/// daemonProxy lets the development server stand in for the daemon when
/// the app is opened in a browser at http://localhost:1420 (ADR-0035).
///
/// There, the app calls the API at the page's own origin, as it does when
/// the daemon serves it. This passes each /v1/ request on to whichever
/// daemon the discovery file names at that moment, so a daemon `make dev`
/// restarts on a new port is followed without restarting Vite. The desktop
/// in `tauri dev` calls the daemon directly and never comes through here.
export function daemonProxy(): Plugin {
  return {
    name: "jobpilot-daemon-proxy",
    apply: "serve",
    configureServer(server) {
      server.middlewares.use("/v1", (req, res) => {
        const port = daemonPort();
        if (port === null) {
          res.writeHead(502, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "no JobPilot daemon is running; start one with `make dev`" }));
          return;
        }
        const upstream = http.request(
          {
            host: "127.0.0.1",
            port,
            method: req.method,
            // Connect strips the mount path from req.url.
            path: `/v1${req.url ?? ""}`,
            // The daemon answers only requests that name the loopback
            // address (ADR-0035 §3).
            headers: { ...req.headers, host: `127.0.0.1:${port}` },
          },
          (answer) => {
            res.writeHead(answer.statusCode ?? 502, answer.headers);
            answer.pipe(res);
          },
        );
        upstream.on("error", () => {
          if (res.headersSent) {
            res.destroy();
            return;
          }
          res.writeHead(502, { "Content-Type": "application/json" });
          res.end(JSON.stringify({ error: "could not reach the JobPilot daemon; it may have stopped" }));
        });
        req.pipe(upstream);
      });
    },
  };
}

/// daemonPort reads the discovery file the daemon writes into its data
/// directory, which is os.UserConfigDir()/jobpilot (internal/adapters/
/// secondary/system/paths.go), or null when there is none.
function daemonPort(): number | null {
  try {
    const info = JSON.parse(readFileSync(path.join(userConfigDir(), "jobpilot", "daemon.json"), "utf8"));
    return typeof info.port === "number" ? info.port : null;
  } catch {
    return null;
  }
}

/// userConfigDir is Go's os.UserConfigDir.
function userConfigDir(): string {
  switch (process.platform) {
    case "darwin":
      return path.join(os.homedir(), "Library", "Application Support");
    case "win32":
      return process.env.AppData ?? path.join(os.homedir(), "AppData", "Roaming");
    default:
      return process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config");
  }
}
