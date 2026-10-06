package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// registerPrompts serves the guidance the user gives the prompts they may
// change (ADR-0031 §8).
func registerPrompts(mux *http.ServeMux, svc *services.SettingsService) {
	mux.HandleFunc("GET /v1/settings/prompts", handleListPromptGuidance(svc))
	mux.HandleFunc("PUT /v1/settings/prompts/{key}", handleSavePromptGuidance(svc))
	mux.HandleFunc("DELETE /v1/settings/prompts/{key}", handleResetPromptGuidance(svc))
}

// PromptGuidanceList is the body of GET /v1/settings/prompts. It carries
// the longest guidance core keeps, so a client can say so as it is typed
// rather than hold a copy of the number.
type PromptGuidanceList struct {
	Prompts   []domain.PromptGuidance `json:"prompts"`
	MaxLength int                     `json:"max_length"`
}

// promptGuidanceRequest is the body of PUT /v1/settings/prompts/{key}.
type promptGuidanceRequest struct {
	Text string `json:"text"`
}

// promptGuidanceStatus is the HTTP status for a prompt guidance error:
// guidance that cannot be kept is the client's, and a key that is not a
// prompt is a 404, as any record the URL names is.
func promptGuidanceStatus(err error) int {
	if errors.Is(err, domain.ErrInvalidGuidance) {
		return http.StatusBadRequest
	}
	return fallbackStatus(err)
}

func handleListPromptGuidance(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prompts, err := svc.ListPromptGuidance(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, PromptGuidanceList{Prompts: prompts, MaxLength: domain.MaxGuidanceLength})
	}
}

func handleSavePromptGuidance(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req promptGuidanceRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		guidance, err := svc.SavePromptGuidance(r.Context(), domain.PromptKey(r.PathValue("key")), req.Text)
		if err != nil {
			writeError(w, promptGuidanceStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, guidance)
	}
}

func handleResetPromptGuidance(svc *services.SettingsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guidance, err := svc.ResetPromptGuidance(r.Context(), domain.PromptKey(r.PathValue("key")))
		if err != nil {
			writeError(w, promptGuidanceStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, guidance)
	}
}

// ListPromptGuidance returns every prompt the user may give guidance to,
// with what they gave it.
func (c *Client) ListPromptGuidance(ctx context.Context) ([]domain.PromptGuidance, error) {
	var body PromptGuidanceList
	if _, err := c.do(ctx, http.MethodGet, "/v1/settings/prompts", nil, &body); err != nil {
		return nil, err
	}
	return body.Prompts, nil
}

// SavePromptGuidance replaces a prompt's guidance with the user's.
func (c *Client) SavePromptGuidance(ctx context.Context, key domain.PromptKey, text string) (*domain.PromptGuidance, error) {
	var guidance domain.PromptGuidance
	path := "/v1/settings/prompts/" + url.PathEscape(string(key))
	if _, err := c.do(ctx, http.MethodPut, path, promptGuidanceRequest{Text: text}, &guidance); err != nil {
		return nil, err
	}
	return &guidance, nil
}

// ResetPromptGuidance puts a prompt back on its default guidance.
func (c *Client) ResetPromptGuidance(ctx context.Context, key domain.PromptKey) (*domain.PromptGuidance, error) {
	var guidance domain.PromptGuidance
	path := "/v1/settings/prompts/" + url.PathEscape(string(key))
	if _, err := c.do(ctx, http.MethodDelete, path, nil, &guidance); err != nil {
		return nil, err
	}
	return &guidance, nil
}
