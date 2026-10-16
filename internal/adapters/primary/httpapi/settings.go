package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

func handleSetupEnvironment(svc *services.OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, err := svc.SetupEnvironment(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"data_dir": dir})
	}
}

func handleConfigureLLM(svc *services.OnboardingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := svc.ConfigureLLM(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func handleGetDiscoverySettings(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.GetDiscoverySettings(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func handleSaveDiscoverySettings(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var settings domain.DiscoverySettings
		if err := readJSON(r, &settings); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := svc.SaveDiscoverySettings(r.Context(), &settings); writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func handleGetLLMSettings(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.GetLLMSettings(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

// saveLLMSettingsResponse carries the saved settings back, plus a
// non-fatal connectivity warning: saving must succeed even if the new
// backend can't be reached right now, matching the CLI's prior in-process
// behavior of warning rather than failing in that case.
type saveLLMSettingsResponse struct {
	Settings          domain.LLMSettings `json:"settings"`
	ConnectivityError string             `json:"connectivity_error,omitempty"`
}

func handleSaveLLMSettings(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var settings domain.LLMSettings
		if err := readJSON(r, &settings); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if err := svc.Settings.SaveLLMSettings(r.Context(), &settings); writeErrIfAny(w, err) {
			return
		}

		resp := saveLLMSettingsResponse{Settings: settings}
		if err := svc.LLM.TestConnectivity(r.Context()); err != nil {
			resp.ConnectivityError = err.Error()
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// llmHealthCheckTimeout bounds the connectivity probe well under the
// providers' own 30s request timeout: the probe runs before a UI action
// starts, so an unreachable host must fail fast rather than leave the user
// waiting on a button.
const llmHealthCheckTimeout = 5 * time.Second

// llmHealthResponse reports whether a generation request would work right
// now. Configured and OK are separate because the two failures need
// different advice: nothing has been set up yet, versus a backend that is
// set up but not answering.
type llmHealthResponse struct {
	Configured bool   `json:"configured"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

// handleLLMHealth lets a caller check the configured backend before starting
// work that depends on it, rather than discovering the problem partway
// through and leaving the user with half a result.
func handleLLMHealth(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.Settings.GetLLMSettings(r.Context())
		if writeErrIfAny(w, err) {
			return
		}

		// Unconfigured settings resolve to the Ollama defaults, which may
		// well answer — but the user still has to choose a model that is
		// actually installed, so there is nothing to probe for yet.
		if settings.Provider == "" {
			writeJSON(w, http.StatusOK, llmHealthResponse{})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), llmHealthCheckTimeout)
		defer cancel()

		resp := llmHealthResponse{Configured: true}
		if err := svc.LLM.TestConnectivity(ctx); err != nil {
			resp.Error = err.Error()
		} else {
			resp.OK = true
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
