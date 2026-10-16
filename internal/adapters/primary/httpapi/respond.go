package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/unicdev/jobpilot/internal/core/ports"
)

// intQueryParam reads an optional non-negative integer query parameter.
// An absent or empty parameter yields 0, which callers read as "unset".
func intQueryParam(query url.Values, name string) (int, error) {
	raw := query.Get(name)
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be a whole number", name, raw)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid %s %d: must not be negative", name, value)
	}
	return value, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// writeErrIfAny writes an error response and returns true if err is non-nil,
// so handlers can do `if writeErrIfAny(w, err) { return }`. The status is
// fallbackStatus's.
func writeErrIfAny(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	writeError(w, fallbackStatus(err), err)
	return true
}

// fallbackStatus is the status for an error no handler gave a meaning of its
// own: 404 for a record the URL names that does not exist, 422 for one the
// request body names, and 500 for anything else. Every handler's error ends
// here when nothing more specific applies, so a missing job or application
// is never reported as the daemon failing.
func fallbackStatus(err error) int {
	switch {
	case errors.Is(err, ports.ErrUnknownReference):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ports.ErrNotFound):
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func readJSON(r *http.Request, dest any) error {
	defer func() { _ = r.Body.Close() }()
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}
