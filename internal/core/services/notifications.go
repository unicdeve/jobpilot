package services

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// notificationSettings is what NotificationService needs of the settings.
type notificationSettings interface {
	GetNotificationSettings(ctx context.Context) (*domain.NotificationSettings, error)
	SaveNotificationSettings(ctx context.Context, settings *domain.NotificationSettings) error
	GetDesktopExecutable(ctx context.Context) (string, error)
	SaveDesktopExecutable(ctx context.Context, path string) error
}

// deliveryTimeout bounds starting the desktop app to post notifications.
const deliveryTimeout = 30 * time.Second

// recordTimeout bounds recording a notification as a run finishes, on a
// context the run's cancellation does not reach.
const recordTimeout = 5 * time.Second

// NotificationService decides what is worth telling the user, keeps it,
// and has the desktop app post it (ADR-0028).
type NotificationService struct {
	repo     ports.NotificationRepository
	settings notificationSettings
	// desktop is nil where no desktop app can be started, as in tests;
	// notifications are still kept and shown in the app.
	desktop ports.DesktopApp
	now     func() time.Time

	// deliveryMu holds one delivery at a time, and deliveries lets the
	// daemon wait for one in progress before it stops.
	deliveryMu sync.Mutex
	deliveries sync.WaitGroup
}

// NewNotificationService constructs a NotificationService.
func NewNotificationService(repo ports.NotificationRepository, settings notificationSettings,
	desktop ports.DesktopApp) *NotificationService {
	return &NotificationService{repo: repo, settings: settings, desktop: desktop, now: time.Now}
}

// ScoutRunFinished is told how every scout run ended. It records a
// notification if the run is worth telling about (domain.ScoutRunNotification),
// and asks the desktop app to post it, in the background.
//
// It never fails the run: a run's result stands whether or not anyone was
// told, so a failure here is logged (ADR-0028 §7).
func (s *NotificationService) ScoutRunFinished(ctx context.Context, run domain.ScoutRunOutcome) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()

	settings, err := s.settings.GetNotificationSettings(ctx)
	if err != nil {
		slog.WarnContext(ctx, "failed to read notification settings", "error", err)
		defaults := domain.DefaultNotificationSettings()
		settings = &defaults
	}
	notification := domain.ScoutRunNotification(run, settings.StrongMatchScore)
	if notification == nil {
		return
	}
	notification.CreatedAt = s.now()
	if err := s.repo.Save(ctx, notification); err != nil {
		slog.WarnContext(ctx, "failed to record a notification", "scout_id", run.ScoutID, "error", err)
		return
	}
	s.deliverInBackground()
}

// deliverInBackground has the desktop app post what is waiting, on a
// context of its own: the run that recorded it is over.
func (s *NotificationService) deliverInBackground() {
	s.deliveries.Add(1)
	go func() {
		defer s.deliveries.Done()
		ctx, cancel := context.WithTimeout(context.Background(), deliveryTimeout)
		defer cancel()
		if err := s.Deliver(ctx); err != nil {
			slog.WarnContext(ctx, "failed to have the desktop app post notifications", "error", err)
		}
	}()
}

// Deliver starts the desktop app to post the notifications waiting to be
// posted, if there are any, the user has not turned notifications off,
// and the app has said where it is. Otherwise the notifications wait in
// the app.
func (s *NotificationService) Deliver(ctx context.Context) error {
	if s.desktop == nil {
		return nil
	}
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()

	waiting, err := s.Postable(ctx)
	if err != nil || len(waiting) == 0 {
		return err
	}
	executable, err := s.settings.GetDesktopExecutable(ctx)
	if err != nil || executable == "" {
		return err
	}
	return s.desktop.DeliverNotifications(ctx, executable)
}

// Wait blocks until a delivery in progress has finished.
func (s *NotificationService) Wait() {
	s.deliveries.Wait()
}

// Postable returns the notifications the desktop app should post now:
// none when the user turned notifications off, and none recorded too long
// ago to be news (domain.Notification.Postable).
func (s *NotificationService) Postable(ctx context.Context) ([]*domain.Notification, error) {
	settings, err := s.settings.GetNotificationSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return []*domain.Notification{}, nil
	}
	now := s.now()
	recent, err := s.repo.ListUnposted(ctx, now.Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	postable := make([]*domain.Notification, 0, len(recent))
	for _, notification := range recent {
		if notification.Postable(now) {
			postable = append(postable, notification)
		}
	}
	return postable, nil
}

// ClaimPosted records that a notification is being posted, and reports
// whether this caller may post it. Only the first caller may.
func (s *NotificationService) ClaimPosted(ctx context.Context, id string) (bool, error) {
	return s.repo.ClaimPosted(ctx, id, s.now())
}

// List returns notifications newest first, only those not yet seen in the
// app when unseen is set.
func (s *NotificationService) List(ctx context.Context, unseen bool, limit int) ([]*domain.Notification, error) {
	return s.repo.List(ctx, unseen, limit)
}

// MarkSeen records that the user saw a notification in JobPilot, or every
// one when id is empty, and returns how many it marked.
func (s *NotificationService) MarkSeen(ctx context.Context, id string) (int, error) {
	return s.repo.MarkSeen(ctx, id, s.now())
}

// Settings returns the user's choices about notifications.
func (s *NotificationService) Settings(ctx context.Context) (*domain.NotificationSettings, error) {
	return s.settings.GetNotificationSettings(ctx)
}

// SaveSettings validates and keeps the user's choices about notifications.
func (s *NotificationService) SaveSettings(ctx context.Context, settings *domain.NotificationSettings) error {
	return s.settings.SaveNotificationSettings(ctx, settings)
}

// RegisterDesktop records where the desktop app's executable is. The app
// says so each time it starts, which is how the daemon finds it to post a
// notification, installed or under `tauri dev` (ADR-0028 §1).
func (s *NotificationService) RegisterDesktop(ctx context.Context, executable string) error {
	return s.settings.SaveDesktopExecutable(ctx, executable)
}
