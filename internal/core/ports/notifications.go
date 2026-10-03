package ports

import (
	"context"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
)

// NotificationRepository keeps what JobPilot told the user, or will
// (ADR-0028 §4).
type NotificationRepository interface {
	Save(ctx context.Context, notification *domain.Notification) error
	// List returns notifications newest first: only those the user has not
	// seen when unseen is set. A limit of zero or less is the
	// repository's default.
	List(ctx context.Context, unseen bool, limit int) ([]*domain.Notification, error)
	// ListUnposted returns, oldest first, those never given to the
	// operating system and recorded at or after since.
	ListUnposted(ctx context.Context, since time.Time) ([]*domain.Notification, error)
	// ClaimPosted records that a notification is being posted, and
	// reports whether this caller claimed it. Only the first caller does,
	// so two deliveries never post one notification twice.
	ClaimPosted(ctx context.Context, id string, at time.Time) (bool, error)
	// MarkSeen records that the user saw one notification, or every one
	// not yet seen when id is empty. It returns how many it marked.
	MarkSeen(ctx context.Context, id string, at time.Time) (int, error)
}

// DesktopApp is the desktop application, as the daemon reaches it: the
// one thing that can post a notification under JobPilot's own name
// (ADR-0028 §1).
type DesktopApp interface {
	// DeliverNotifications starts the app at executable in its delivery
	// mode, which posts the notifications not yet posted and exits. It
	// returns once the app has started, not once it has posted.
	DeliverNotifications(ctx context.Context, executable string) error
}
