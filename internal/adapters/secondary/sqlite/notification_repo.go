package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// NotificationRepository keeps notifications in SQLite (ADR-0028 §4).
type NotificationRepository struct {
	db *sql.DB
}

// NewNotificationRepository constructs a NotificationRepository over db.
func NewNotificationRepository(db *sql.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

var _ ports.NotificationRepository = (*NotificationRepository)(nil)

// defaultNotificationLimit is how many notifications a list returns when
// the caller does not say.
const defaultNotificationLimit = 50

const notificationColumns = `
	id, kind, title, body, scout_id, run_id, created_at, posted_at, seen_at`

// Save inserts a new notification, assigning its ID.
func (r *NotificationRepository) Save(ctx context.Context, notification *domain.Notification) error {
	if notification.ID == "" {
		notification.ID = uuid.NewString()
	}
	if notification.CreatedAt.IsZero() {
		notification.CreatedAt = time.Now()
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO notifications (`+notificationColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		notification.ID, string(notification.Kind), notification.Title, notification.Body,
		nullableString(notification.ScoutID), nullableString(notification.RunID),
		notification.CreatedAt, notification.PostedAt, notification.SeenAt)
	if err != nil {
		return fmt.Errorf("failed to save notification: %w", err)
	}
	return nil
}

func scanNotification(scanner interface{ Scan(...any) error }) (*domain.Notification, error) {
	var n domain.Notification
	var kind string
	if err := scanner.Scan(&n.ID, &kind, &n.Title, &n.Body, nullStringInto(&n.ScoutID), nullStringInto(&n.RunID),
		&n.CreatedAt, &n.PostedAt, &n.SeenAt); err != nil {
		return nil, err
	}
	n.Kind = domain.NotificationKind(kind)
	return &n, nil
}

func (r *NotificationRepository) query(ctx context.Context, query string, args ...any) ([]*domain.Notification, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	notifications := make([]*domain.Notification, 0)
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read notifications: %w", err)
	}
	return notifications, nil
}

// List returns notifications newest first, only those not yet seen when
// unseen is set.
func (r *NotificationRepository) List(ctx context.Context, unseen bool, limit int) ([]*domain.Notification, error) {
	if limit <= 0 {
		limit = defaultNotificationLimit
	}
	where := ""
	if unseen {
		where = ` WHERE seen_at IS NULL`
	}
	// id breaks ties between notifications recorded in the same instant.
	return r.query(ctx, `SELECT`+notificationColumns+` FROM notifications`+where+
		` ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
}

// ListUnposted returns, oldest first, those never posted and recorded at
// or after since.
func (r *NotificationRepository) ListUnposted(ctx context.Context, since time.Time) ([]*domain.Notification, error) {
	return r.query(ctx, `SELECT`+notificationColumns+` FROM notifications
		WHERE posted_at IS NULL AND created_at >= ? ORDER BY created_at, id`, since)
}

// ClaimPosted records that a notification is being posted, and reports
// whether this caller claimed it: the update only matches one not yet
// posted, so a second caller changes nothing and is told so.
func (r *NotificationRepository) ClaimPosted(ctx context.Context, id string, at time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET posted_at = ? WHERE id = ? AND posted_at IS NULL`, at, id)
	if err != nil {
		return false, fmt.Errorf("failed to record a notification as posted: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to record a notification as posted: %w", err)
	}
	if affected == 1 {
		return true, nil
	}
	// Nothing changed: either it was posted already, or there is no such
	// notification, which is the caller's to hear about.
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM notifications WHERE id = ?`, id).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to read notification: %w", err)
	}
	if exists == 0 {
		return false, &ports.NotFoundError{Kind: "notification", ID: id}
	}
	return false, nil
}

// MarkSeen records that the user saw one notification, or every one not
// yet seen when id is empty, and returns how many it marked. Marking one
// already seen marks nothing and is not an error.
func (r *NotificationRepository) MarkSeen(ctx context.Context, id string, at time.Time) (int, error) {
	query, args := `UPDATE notifications SET seen_at = ? WHERE seen_at IS NULL`, []any{at}
	if id != "" {
		var exists int
		if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM notifications WHERE id = ?`, id).Scan(&exists); err != nil {
			return 0, fmt.Errorf("failed to read notification: %w", err)
		}
		if exists == 0 {
			return 0, &ports.NotFoundError{Kind: "notification", ID: id}
		}
		query, args = query+` AND id = ?`, append(args, id)
	}
	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to mark notifications seen: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to mark notifications seen: %w", err)
	}
	return int(affected), nil
}
