package httpapi

import (
	"errors"
	"net/http"

	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// setCredentialRequest carries the secret being stored. It has no response
// counterpart: nothing returns a secret.
type setCredentialRequest struct {
	Secret string `json:"secret"`
}

// handleListCredentials serves which credentials are configured, and no
// values. The response is refs only, which are safe to show and to log.
func handleListCredentials(svc *services.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refs, err := svc.List(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, refs)
	}
}

func handleSetCredential(svc *services.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setCredentialRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		ref, err := svc.Set(r.Context(), r.PathValue("source"), r.PathValue("label"), req.Secret)
		if err != nil {
			// A malformed ref and an empty secret are the caller's to
			// correct; a keychain that never answered is not, and telling
			// the user to fix a correct request would send them looking in
			// the wrong place. The error names the ref and never the
			// secret either way.
			writeError(w, credentialStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, ref)
	}
}

func handleDeleteCredential(svc *services.CredentialService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Delete(r.Context(), r.PathValue("source"), r.PathValue("label")); err != nil {
			writeError(w, credentialStatus(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// credentialStatus separates a request the caller should fix from a
// keystore that could not be reached.
func credentialStatus(err error) int {
	if errors.Is(err, ports.ErrCredentialStoreUnavailable) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}
