package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// registerNotifications serves what JobPilot told the user, the desktop
// app's delivery of it, and the user's choices about it (ADR-0028).
func registerNotifications(mux *http.ServeMux, svc *services.NotificationService) {
	mux.HandleFunc("GET /v1/notifications", handleListNotifications(svc))
	mux.HandleFunc("GET /v1/notifications/postable", handlePostableNotifications(svc))
	mux.HandleFunc("POST /v1/notifications/{id}/posted", handleClaimPosted(svc))
	mux.HandleFunc("POST /v1/notifications/{id}/seen", handleMarkSeen(svc))
	mux.HandleFunc("POST /v1/notifications/seen", handleMarkSeen(svc))
	mux.HandleFunc("GET /v1/settings/notifications", handleGetNotificationSettings(svc))
	mux.HandleFunc("PUT /v1/settings/notifications", handleSaveNotificationSettings(svc))
	mux.HandleFunc("PUT /v1/desktop", handleRegisterDesktop(svc))
}

// Notifications is the body of the notification lists.
type Notifications struct {
	Notifications []*domain.Notification `json:"notifications"`
}

// Claim is the body of POST /v1/notifications/{id}/posted: whether this
// caller may post it. Only the first caller may.
type Claim struct {
	Claimed bool `json:"claimed"`
}

// Marked is the body of POST /v1/notifications/.../seen.
type Marked struct {
	Marked int `json:"marked"`
}

// desktopRequest is what the desktop app says about itself when it starts.
type desktopRequest struct {
	Executable string `json:"executable"`
}

func handleListNotifications(svc *services.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		unseen := r.URL.Query().Get("unseen") == "true"
		notifications, err := svc.List(r.Context(), unseen, limit)
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, Notifications{Notifications: notifications})
	}
}

// handlePostableNotifications is what the desktop app asks in its
// delivery mode: what to post now.
func handlePostableNotifications(svc *services.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		notifications, err := svc.Postable(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, Notifications{Notifications: notifications})
	}
}

func handleClaimPosted(svc *services.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claimed, err := svc.ClaimPosted(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, Claim{Claimed: claimed})
	}
}

// handleMarkSeen marks one notification seen, or every one when the path
// names none.
func handleMarkSeen(svc *services.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		marked, err := svc.MarkSeen(r.Context(), r.PathValue("id"))
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, Marked{Marked: marked})
	}
}

func handleGetNotificationSettings(svc *services.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.Settings(r.Context())
		if writeErrIfAny(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func handleSaveNotificationSettings(svc *services.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var settings domain.NotificationSettings
		if err := readJSON(r, &settings); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := settings.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if writeErrIfAny(w, svc.SaveSettings(r.Context(), &settings)) {
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func handleRegisterDesktop(svc *services.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req desktopRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := svc.RegisterDesktop(r.Context(), req.Executable); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListNotifications returns notifications newest first, only those not
// yet seen when unseen is set.
func (c *Client) ListNotifications(ctx context.Context, unseen bool) ([]*domain.Notification, error) {
	path := "/v1/notifications"
	if unseen {
		path += "?" + url.Values{"unseen": {"true"}}.Encode()
	}
	var body Notifications
	if _, err := c.do(ctx, http.MethodGet, path, nil, &body); err != nil {
		return nil, err
	}
	return body.Notifications, nil
}

// MarkNotificationsSeen marks one notification seen, or every one when id
// is empty, and returns how many it marked.
func (c *Client) MarkNotificationsSeen(ctx context.Context, id string) (int, error) {
	path := "/v1/notifications/seen"
	if id != "" {
		path = "/v1/notifications/" + url.PathEscape(id) + "/seen"
	}
	var body Marked
	if _, err := c.do(ctx, http.MethodPost, path, nil, &body); err != nil {
		return 0, err
	}
	return body.Marked, nil
}

// GetNotificationSettings returns the user's choices about notifications.
func (c *Client) GetNotificationSettings(ctx context.Context) (*domain.NotificationSettings, error) {
	var settings domain.NotificationSettings
	if _, err := c.do(ctx, http.MethodGet, "/v1/settings/notifications", nil, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

// SaveNotificationSettings keeps the user's choices about notifications.
func (c *Client) SaveNotificationSettings(ctx context.Context, settings *domain.NotificationSettings) error {
	_, err := c.do(ctx, http.MethodPut, "/v1/settings/notifications", settings, nil)
	return err
}
