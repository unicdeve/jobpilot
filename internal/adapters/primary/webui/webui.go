// Package webui serves JobPilot's interface to a web browser: the
// desktop's React app, built and compiled into the daemon, at the same
// origin as the API it calls (ADR-0035).
package webui

import (
	"io/fs"
	"net/http"
	"strings"
)

// contentSecurityPolicy is the desktop's (tauri.conf.json), with the page
// talking only to its own origin, and kept out of other websites' frames:
// a page that can be framed can be clicked through by a page that frames
// it.
const contentSecurityPolicy = "default-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

// Built reports whether this binary carries the interface, which only a
// build with the webui tag does.
func Built() bool { return files() != nil }

// Handler serves the interface, or, in a binary built without it, a page
// that says how to build one with it.
func Handler() http.Handler {
	fsys := files()
	if fsys == nil {
		return http.HandlerFunc(notBuilt)
	}
	return serve(fsys)
}

func serve(fsys fs.FS) http.Handler {
	files := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		header := w.Header()
		header.Set("Content-Security-Policy", contentSecurityPolicy)
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		// Vite names every asset after its content, so one never changes
		// under its name. The page that names them does, with each build.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			header.Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func notBuilt(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("This JobPilot daemon was built without its interface.\n\n" +
		"A release carries it. In a checkout, build it in with `make build-web`, then run `jobpilot-cli ui`.\n"))
}
