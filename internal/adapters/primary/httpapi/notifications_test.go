package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// memoryNotifications is an in-memory ports.NotificationRepository.
type memoryNotifications struct {
	mu    sync.Mutex
	items []*domain.Notification
}

func (m *memoryNotifications) Save(_ context.Context, n *domain.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n.ID = "n" + strconv.Itoa(len(m.items)+1)
	stored := *n
	m.items = append(m.items, &stored)
	return nil
}

func (m *memoryNotifications) List(_ context.Context, unseen bool, _ int) ([]*domain.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*domain.Notification{}
	for i := len(m.items) - 1; i >= 0; i-- {
		if !unseen || m.items[i].SeenAt == nil {
			read := *m.items[i]
			out = append(out, &read)
		}
	}
	return out, nil
}

func (m *memoryNotifications) ListUnposted(_ context.Context, since time.Time) ([]*domain.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*domain.Notification{}
	for _, n := range m.items {
		if n.PostedAt == nil && !n.CreatedAt.Before(since) {
			read := *n
			out = append(out, &read)
		}
	}
	return out, nil
}

func (m *memoryNotifications) ClaimPosted(_ context.Context, id string, at time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.items {
		if n.ID == id {
			if n.PostedAt != nil {
				return false, nil
			}
			n.PostedAt = &at
			return true, nil
		}
	}
	return false, &ports.NotFoundError{Kind: "notification", ID: id}
}

func (m *memoryNotifications) MarkSeen(_ context.Context, id string, at time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	marked, found := 0, id == ""
	for _, n := range m.items {
		if id != "" && n.ID != id {
			continue
		}
		found = true
		if n.SeenAt == nil {
			n.SeenAt = &at
			marked++
		}
	}
	if !found {
		return 0, &ports.NotFoundError{Kind: "notification", ID: id}
	}
	return marked, nil
}

func notificationServer(t *testing.T) (*Client, *services.NotificationService) {
	t.Helper()
	svc := services.NewNotificationService(&memoryNotifications{},
		services.NewSettingsService(newFakeSettingsRepository()), nil)
	ts := httptest.NewServer(NewHandler(Services{Notifications: svc}))
	t.Cleanup(ts.Close)
	_, portStr, err := net.SplitHostPort(ts.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return NewClient(port), svc
}

func TestServer_Notifications(t *testing.T) {
	client, svc := notificationServer(t)
	ctx := context.Background()
	svc.ScoutRunFinished(ctx, domain.ScoutRunOutcome{
		ScoutID: "scout-1", ScoutName: "Mornings", RunID: "run-1", Trigger: domain.RunTriggerSchedule,
		Report: &domain.ScoutRunReport{Verdicts: []domain.ScoutVerdict{{Eligible: true, HasScore: true, Score: 90}}},
	})

	unseen, err := client.ListNotifications(ctx, true)
	require.NoError(t, err)
	require.Len(t, unseen, 1)
	assert.Equal(t, "Mornings found 1 job for you", unseen[0].Title)

	// What the desktop app's delivery mode asks, and claims.
	var postable Notifications
	_, err = client.do(ctx, http.MethodGet, "/v1/notifications/postable", nil, &postable)
	require.NoError(t, err)
	require.Len(t, postable.Notifications, 1)
	var claim Claim
	_, err = client.do(ctx, http.MethodPost, "/v1/notifications/"+unseen[0].ID+"/posted", nil, &claim)
	require.NoError(t, err)
	assert.True(t, claim.Claimed)
	_, err = client.do(ctx, http.MethodPost, "/v1/notifications/"+unseen[0].ID+"/posted", nil, &claim)
	require.NoError(t, err)
	assert.False(t, claim.Claimed, "a second delivery does not post it again")
	status, _ := requestError(t, client, http.MethodPost, "/v1/notifications/missing/posted", nil)
	assert.Equal(t, http.StatusNotFound, status)

	marked, err := client.MarkNotificationsSeen(ctx, unseen[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 1, marked)
	unseen, err = client.ListNotifications(ctx, true)
	require.NoError(t, err)
	assert.Empty(t, unseen)
	all, err := client.ListNotifications(ctx, false)
	require.NoError(t, err)
	assert.Len(t, all, 1, "seen is kept, not deleted")
}

func TestServer_NotificationSettings(t *testing.T) {
	client, _ := notificationServer(t)
	ctx := context.Background()

	settings, err := client.GetNotificationSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, domain.DefaultNotificationSettings(), *settings)

	require.NoError(t, client.SaveNotificationSettings(ctx, &domain.NotificationSettings{Enabled: false, StrongMatchScore: 75}))
	settings, err = client.GetNotificationSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, domain.NotificationSettings{Enabled: false, StrongMatchScore: 75}, *settings)

	status, message := requestError(t, client, http.MethodPut, "/v1/settings/notifications",
		map[string]any{"enabled": true, "strong_match_score": 0})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, message, "between 1 and 100")
}

func TestServer_RegisterDesktop(t *testing.T) {
	client, _ := notificationServer(t)

	status, message := requestError(t, client, http.MethodPut, "/v1/desktop", map[string]any{"executable": "relative/app"})
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, message, "absolute path")

	_, err := client.do(context.Background(), http.MethodPut, "/v1/desktop",
		desktopRequest{Executable: "/Applications/JobPilot.app/Contents/MacOS/jobpilot-desktop"}, nil)
	assert.NoError(t, err)
}
