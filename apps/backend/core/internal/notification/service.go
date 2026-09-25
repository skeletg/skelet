package notification

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// maxBodyLen caps a notification body (e.g. a job's last_error) so it stays
// scannable in the inbox
const maxBodyLen = 240

// CfgRetentionDays drives the daily notifications-prune job (read ones; never-read
// notifications get a 6x backstop so an unopened inbox stays bounded too); <= 0 disables
const CfgRetentionDays = "system/notifications/retention_days"

func init() {
	config.RegisterGroup(config.Group{Section: "system", Slug: "notifications", Label: "Notifications", SortOrder: 3})
	config.Register(config.Definition{Path: CfgRetentionDays, Type: config.TypeInt, Default: "30", Label: "Notification retention after read (days)"})
}

type Service struct {
	q   *db.Queries
	hub *Hub
}

// hub may be nil on tests and CLI, leaving notifications inbox-only with no live push
func New(q *db.Queries, hub *Hub) *Service { return &Service{q: q, hub: hub} }

// Inserts a notification deduped by DedupeKey, where a repeat is a no-op, and pushes genuinely new ones to live subscription listeners through the hub
func (s *Service) Create(ctx context.Context, in Input) error {
	sev := in.Severity
	if sev == "" {
		sev = SeverityInfo
	}
	var ch uuid.NullUUID
	if in.ChannelID != nil {
		ch = uuid.NullUUID{UUID: *in.ChannelID, Valid: true}
	}
	row, err := s.q.CreateNotification(ctx, db.CreateNotificationParams{
		Category:           in.Category,
		Severity:           sev,
		Title:              in.Title,
		Body:               in.Body,
		RequiredPermission: in.RequiredPermission,
		ChannelID:          ch,
		EntityType:         in.EntityType,
		EntityID:           in.EntityID,
		ActionUrl:          in.ActionURL,
		DedupeKey:          in.DedupeKey,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil // dedupe hit: already in the inbox, no re-push
	}
	if err != nil {
		return err
	}
	s.hub.Publish(ctx, Notification{
		ID: row.ID, Category: in.Category, Severity: sev, Title: in.Title, Body: in.Body,
		ChannelID: in.ChannelID, EntityType: in.EntityType, EntityID: in.EntityID,
		ActionURL: in.ActionURL, CreatedAt: row.CreatedAt,
		RequiredPermission: in.RequiredPermission,
	})
	return nil
}

// The notifications visible to the admin, newest first
func (s *Service) List(ctx context.Context, sc Scope, unreadOnly bool, limit int32) ([]Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.q.ListNotifications(ctx, db.ListNotificationsParams{
		AdminID:      sc.AdminID,
		Bypass:       sc.Bypass,
		Perms:        sc.Perms,
		GlobalPerms:  sc.GlobalPerms,
		PermChannels: sc.permChannelPairs(),
		Muted:        sc.Muted,
		UnreadOnly:   unreadOnly,
		Lim:          limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Notification, 0, len(rows))
	for _, r := range rows {
		read, _ := r.IsRead.(bool)
		var ch *uuid.UUID
		if r.ChannelID.Valid {
			c := r.ChannelID.UUID
			ch = &c
		}
		out = append(out, Notification{
			ID: r.ID, Category: r.Category, Severity: r.Severity, Title: r.Title, Body: r.Body,
			ChannelID: ch, EntityType: r.EntityType, EntityID: r.EntityID, ActionURL: r.ActionUrl,
			CreatedAt: r.CreatedAt, Read: read,
		})
	}
	return out, nil
}

// The badge number - visible and unread for this admin
func (s *Service) UnreadCount(ctx context.Context, sc Scope) (int64, error) {
	return s.q.CountUnreadNotifications(ctx, db.CountUnreadNotificationsParams{
		AdminID:      sc.AdminID,
		Bypass:       sc.Bypass,
		Perms:        sc.Perms,
		GlobalPerms:  sc.GlobalPerms,
		PermChannels: sc.permChannelPairs(),
		Muted:        sc.Muted,
	})
}

func (s *Service) MarkRead(ctx context.Context, adminID, notificationID uuid.UUID) error {
	return s.q.MarkNotificationRead(ctx, db.MarkNotificationReadParams{NotificationID: notificationID, AdminID: adminID})
}

func (s *Service) MarkAllRead(ctx context.Context, sc Scope) error {
	return s.q.MarkAllNotificationsRead(ctx, db.MarkAllNotificationsReadParams{
		AdminID:      sc.AdminID,
		Bypass:       sc.Bypass,
		Perms:        sc.Perms,
		GlobalPerms:  sc.GlobalPerms,
		PermChannels: sc.permChannelPairs(),
		Muted:        sc.Muted,
	})
}

// Posts inbox notifications for standing ops conditions, run on the notification-scan schedule - all writes are deduped, so re-scanning the same condition is a no-op
// v1 covers failing scheduled jobs; offline-payment reconciliation, stale drafts and subscription-renewal failures slot in here
func (s *Service) Scan(ctx context.Context) error {
	jobs, err := s.q.ListFailingScheduledJobs(ctx)
	if err != nil {
		return err
	}
	// This IS the alerting path, so a dropped write means an operator gets no notice that a job is failing AND no notice
	// that alerting itself broke - the one place silence is indistinguishable from health
	var errs []error
	for _, j := range jobs {
		body := "A scheduled job's last run failed - check the scheduler."
		if j.LastError != "" {
			body = "Last run failed: " + truncate(j.LastError, maxBodyLen)
		}
		errs = append(errs, s.Create(ctx, Input{
			Category:           CategorySystem,
			Severity:           SeverityCritical,
			Title:              "Scheduled job failing: " + j.Name,
			Body:               body,
			RequiredPermission: PermSystem,
			EntityType:         "job",
			EntityID:           j.Name,
			ActionURL:          "/scheduled-jobs",
			DedupeKey:          "job_error:" + j.Name,
		}))
	}
	return errors.Join(errs...)
}

// Prune drops read notifications past readBefore plus EVERYTHING past anyBefore -
// the backstop keeps an inbox nobody opens from growing forever (reads cascade)
func (s *Service) Prune(ctx context.Context, readBefore, anyBefore time.Time) (int64, error) {
	read, err := s.q.PruneNotifications(ctx, readBefore)
	if err != nil {
		return read, err
	}
	all, err := s.q.PruneAllNotifications(ctx, anyBefore)
	return read + all, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
