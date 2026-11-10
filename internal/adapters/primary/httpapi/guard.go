package httpapi

import (
	"net"
	"net/http"
)

// loopbackHosts are the names a request to this daemon may give in its
// Host header: the addresses it listens on, and the name that resolves to
// one of them everywhere.
var loopbackHosts = map[string]bool{
	"127.0.0.1": true,
	"localhost": true,
	"::1":       true,
}

// RequireLoopback refuses a request that did not come from this machine's
// own clients, before next sees it (ADR-0035 §3).
//
// Its Host must name the loopback address, on any port. A website that
// points its own name at 127.0.0.1 after its page has loaded (DNS
// rebinding) is then same-origin to the daemon and outside CORS, but its
// requests still name its own host, and are refused here.
//
// It must carry no Origin, or one of the desktop's, or its own: a page
// served by this daemon. A form on any other website can send a POST
// without a preflight, and CORS only keeps it from reading the answer, not
// from shutting the daemon down or deleting data.
//
// The CLI and the desktop's Rust code send no Origin and name 127.0.0.1.
func RequireLoopback(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, errForeignHost)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !desktopOrigins[origin] && origin != "http://"+r.Host {
			writeError(w, http.StatusForbidden, errForeignOrigin)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackHost reports whether host, a Host header with or without a
// port, names the loopback address.
func loopbackHost(host string) bool {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	return loopbackHosts[host]
}

type forbiddenError string

func (e forbiddenError) Error() string { return string(e) }

const (
	errForeignHost   = forbiddenError("this JobPilot daemon only answers requests addressed to 127.0.0.1 or localhost")
	errForeignOrigin = forbiddenError("this JobPilot daemon only answers its own pages and the JobPilot desktop app")
)
