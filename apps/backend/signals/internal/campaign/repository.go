package campaign

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Repository is the persistence seam for campaigns + their recipient log
type Repository interface {
	Create(ctx context.Context, c Campaign) (Campaign, error)
	Get(ctx context.Context, id uuid.UUID) (Campaign, bool, error)
	List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Campaign, error)
	Update(ctx context.Context, c Campaign) (bool, error) // draft-only; ok=false if not a draft
	Schedule(ctx context.Context, id uuid.UUID, when time.Time) (bool, error)
	Cancel(ctx context.Context, id uuid.UUID) (bool, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	Restore(ctx context.Context, id uuid.UUID) error

	ClaimDue(ctx context.Context, limit int32) ([]Campaign, error)
	ListSending(ctx context.Context) ([]Campaign, error)

	// Claimed but never fully materialized - retried until MarkMaterialized lands
	ListUnmaterialized(ctx context.Context) ([]Campaign, error)
	InsertRecipients(ctx context.Context, campaignID uuid.UUID, rows []NewRecipient) error // a recipient already present is left as it is
	MarkMaterialized(ctx context.Context, campaignID uuid.UUID) error
	ListPending(ctx context.Context, campaignID uuid.UUID, limit int32) ([]Recipient, error)
	HasPending(ctx context.Context, campaignID uuid.UUID) (bool, error)
	HasPendingNonHoldout(ctx context.Context, campaignID uuid.UUID) (bool, error)
	// Taken BEFORE the send - false means a concurrent sweep already owns this recipient
	ClaimForSend(ctx context.Context, recipientID uuid.UUID) (bool, error)
	ReleaseClaimFailed(ctx context.Context, recipientID uuid.UUID, msg string) error
	ReleaseClaim(ctx context.Context, recipientID uuid.UUID) error // back to pending, for a send abandoned before any mail went out
	MarkFailed(ctx context.Context, recipientID uuid.UUID, msg string) error
	MarkSkipped(ctx context.Context, recipientID uuid.UUID, msg string) error

	AddCounts(ctx context.Context, campaignID uuid.UUID, sent, failed, skipped int) error // one pass's outcomes
	RefreshCounts(ctx context.Context, campaignID uuid.UUID) error                        // a full recount, for state changes
	Finalize(ctx context.Context, campaignID uuid.UUID) error

	// A/B subject test
	MarkTesting(ctx context.Context, campaignID uuid.UUID) (bool, error)
	ListABTestingDue(ctx context.Context) ([]Campaign, error)
	AssignHoldoutToWinner(ctx context.Context, campaignID uuid.UUID, variant string) error
	SetWinner(ctx context.Context, campaignID uuid.UUID, winner string) (bool, error)

	// Open/click tracking - RecordOpen/RecordClick set the per-recipient first-seen timestamp and report whether this was
	// the first (so the caller bumps the campaign's rolled-up counter exactly once per recipient)
	RecordOpen(ctx context.Context, recipientID uuid.UUID) (firstTime bool, err error)
	RecordClick(ctx context.Context, recipientID uuid.UUID) (firstTime bool, err error)
	BumpOpened(ctx context.Context, campaignID uuid.UUID) error
	BumpClicked(ctx context.Context, campaignID uuid.UUID) error
	// One campaign at a time - the campaign row keeps the counts
	PruneSentRecipients(ctx context.Context, before time.Time) (int64, error)
}

type repo struct {
	q *db.Queries
}

// NewRepository wires the sqlc-backed repository
func NewRepository(q *db.Queries) Repository { return &repo{q: q} }

func (r *repo) Create(ctx context.Context, c Campaign) (Campaign, error) {
	blocks, styles, err := marshalContent(c)
	if err != nil {
		return Campaign{}, err
	}
	row, err := r.q.CreateEmailCampaign(ctx, db.CreateEmailCampaignParams{
		ChannelID:      c.ChannelID,
		Name:           c.Name,
		Subject:        c.Subject,
		Blocks:         blocks,
		Styles:         styles,
		AudienceKind:   c.AudienceKind,
		AudienceID:     nullUUID(c.AudienceID),
		EmailID:        nullUUID(c.EmailID),
		AbEnabled:      c.ABEnabled,
		AbSubjectB:     c.ABSubjectB,
		AbTestPercent:  int32(c.ABTestPercent),
		AbWinnerMetric: c.ABWinnerMetric,
		AbTestMinutes:  int32(c.ABTestMinutes),
	})
	if err != nil {
		return Campaign{}, err
	}
	return toCampaign(row)
}

func (r *repo) Get(ctx context.Context, id uuid.UUID) (Campaign, bool, error) {
	row, err := r.q.GetEmailCampaign(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Campaign{}, false, nil
	}
	if err != nil {
		return Campaign{}, false, err
	}
	c, err := toCampaign(row)
	return c, err == nil, err
}

func (r *repo) List(ctx context.Context, channelID uuid.UUID, includeDeleted bool) ([]Campaign, error) {
	rows, err := r.q.ListEmailCampaigns(ctx, db.ListEmailCampaignsParams{
		ChannelID:      channelID,
		IncludeDeleted: includeDeleted,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Campaign, 0, len(rows))
	for _, row := range rows {
		c, err := toCampaign(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *repo) Update(ctx context.Context, c Campaign) (bool, error) {
	blocks, styles, err := marshalContent(c)
	if err != nil {
		return false, err
	}
	n, err := r.q.UpdateEmailCampaign(ctx, db.UpdateEmailCampaignParams{
		Name:           c.Name,
		Subject:        c.Subject,
		Blocks:         blocks,
		Styles:         styles,
		AudienceKind:   c.AudienceKind,
		AudienceID:     nullUUID(c.AudienceID),
		EmailID:        nullUUID(c.EmailID),
		AbEnabled:      c.ABEnabled,
		AbSubjectB:     c.ABSubjectB,
		AbTestPercent:  int32(c.ABTestPercent),
		AbWinnerMetric: c.ABWinnerMetric,
		AbTestMinutes:  int32(c.ABTestMinutes),
		ID:             c.ID,
	})
	return n > 0, err
}

func (r *repo) Schedule(ctx context.Context, id uuid.UUID, when time.Time) (bool, error) {
	n, err := r.q.ScheduleEmailCampaign(ctx, db.ScheduleEmailCampaignParams{
		ScheduledFor: sql.NullTime{Time: when, Valid: true},
		ID:           id,
	})
	return n > 0, err
}

func (r *repo) Cancel(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := r.q.CancelEmailCampaign(ctx, id)
	return n > 0, err
}

// SoftDelete marks the row deleted: recipients, opens and clicks stay attached
func (r *repo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	n, err := r.q.SoftDeleteEmailCampaign(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("campaign not found")
	}
	return nil
}

func (r *repo) Restore(ctx context.Context, id uuid.UUID) error {
	n, err := r.q.RestoreEmailCampaign(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("campaign not found")
	}
	return nil
}

func (r *repo) ClaimDue(ctx context.Context, limit int32) ([]Campaign, error) {
	rows, err := r.q.ClaimDueCampaigns(ctx, limit)
	if err != nil {
		return nil, err
	}
	return toCampaigns(rows)
}

func (r *repo) ListSending(ctx context.Context) ([]Campaign, error) {
	rows, err := r.q.ListSendingCampaigns(ctx)
	if err != nil {
		return nil, err
	}
	return toCampaigns(rows)
}

func (r *repo) ListUnmaterialized(ctx context.Context) ([]Campaign, error) {
	rows, err := r.q.ListUnmaterializedCampaigns(ctx)
	if err != nil {
		return nil, err
	}
	return toCampaigns(rows)
}

func (r *repo) InsertRecipients(ctx context.Context, campaignID uuid.UUID, rows []NewRecipient) error {
	p := db.InsertCampaignRecipientsParams{CampaignID: campaignID}
	for _, n := range rows {
		p.CustomerIds = append(p.CustomerIds, n.CustomerID)
		p.Emails = append(p.Emails, n.Email)
		p.FirstNames = append(p.FirstNames, n.FirstName)
		p.Variants = append(p.Variants, n.Variant)
		p.Phases = append(p.Phases, n.Phase)
	}
	return r.q.InsertCampaignRecipients(ctx, p)
}

func (r *repo) MarkMaterialized(ctx context.Context, campaignID uuid.UUID) error {
	return r.q.MarkCampaignMaterialized(ctx, campaignID)
}

func (r *repo) HasPendingNonHoldout(ctx context.Context, campaignID uuid.UUID) (bool, error) {
	return r.q.HasPendingNonHoldoutRecipients(ctx, campaignID)
}

func (r *repo) MarkTesting(ctx context.Context, campaignID uuid.UUID) (bool, error) {
	n, err := r.q.MarkCampaignTesting(ctx, campaignID)
	return n > 0, err
}

func (r *repo) ListABTestingDue(ctx context.Context) ([]Campaign, error) {
	rows, err := r.q.ListABTestingDue(ctx)
	if err != nil {
		return nil, err
	}
	return toCampaigns(rows)
}

func (r *repo) AssignHoldoutToWinner(ctx context.Context, campaignID uuid.UUID, variant string) error {
	return r.q.AssignHoldoutToWinner(ctx, db.AssignHoldoutToWinnerParams{Variant: variant, CampaignID: campaignID})
}

func (r *repo) SetWinner(ctx context.Context, campaignID uuid.UUID, winner string) (bool, error) {
	n, err := r.q.SetCampaignWinner(ctx, db.SetCampaignWinnerParams{Winner: winner, ID: campaignID})
	return n > 0, err
}

func (r *repo) ListPending(ctx context.Context, campaignID uuid.UUID, limit int32) ([]Recipient, error) {
	rows, err := r.q.ListPendingRecipients(ctx, db.ListPendingRecipientsParams{CampaignID: campaignID, Lim: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Recipient, len(rows))
	for i, row := range rows {
		out[i] = toRecipient(row)
	}
	return out, nil
}

func (r *repo) HasPending(ctx context.Context, campaignID uuid.UUID) (bool, error) {
	return r.q.HasPendingRecipients(ctx, campaignID)
}

func (r *repo) ClaimForSend(ctx context.Context, recipientID uuid.UUID) (bool, error) {
	n, err := r.q.MarkRecipientSent(ctx, recipientID)
	return n > 0, err
}

func (r *repo) ReleaseClaimFailed(ctx context.Context, recipientID uuid.UUID, msg string) error {
	return r.q.MarkRecipientSendFailed(ctx, db.MarkRecipientSendFailedParams{Error: msg, ID: recipientID})
}

func (r *repo) ReleaseClaim(ctx context.Context, recipientID uuid.UUID) error {
	return r.q.ReleaseRecipientClaim(ctx, recipientID)
}

func (r *repo) MarkFailed(ctx context.Context, recipientID uuid.UUID, msg string) error {
	return r.q.MarkRecipientFailed(ctx, db.MarkRecipientFailedParams{Error: msg, ID: recipientID})
}

func (r *repo) MarkSkipped(ctx context.Context, recipientID uuid.UUID, msg string) error {
	return r.q.MarkRecipientSkipped(ctx, db.MarkRecipientSkippedParams{Error: msg, ID: recipientID})
}

func (r *repo) AddCounts(ctx context.Context, campaignID uuid.UUID, sent, failed, skipped int) error {
	return r.q.AddCampaignCounts(ctx, db.AddCampaignCountsParams{Sent: int32(sent), Failed: int32(failed), Skipped: int32(skipped), ID: campaignID})
}

func (r *repo) RefreshCounts(ctx context.Context, campaignID uuid.UUID) error {
	return r.q.RefreshCampaignCounts(ctx, campaignID)
}

func (r *repo) Finalize(ctx context.Context, campaignID uuid.UUID) error {
	return r.q.FinalizeCampaign(ctx, campaignID)
}

func (r *repo) RecordOpen(ctx context.Context, recipientID uuid.UUID) (bool, error) {
	n, err := r.q.RecordRecipientOpen(ctx, recipientID)
	return n > 0, err
}

func (r *repo) RecordClick(ctx context.Context, recipientID uuid.UUID) (bool, error) {
	n, err := r.q.RecordRecipientClick(ctx, recipientID)
	return n > 0, err
}

func (r *repo) BumpOpened(ctx context.Context, campaignID uuid.UUID) error {
	return r.q.BumpCampaignOpened(ctx, campaignID)
}

func (r *repo) BumpClicked(ctx context.Context, campaignID uuid.UUID) error {
	return r.q.BumpCampaignClicked(ctx, campaignID)
}

// --- mapping helpers ---

func marshalContent(c Campaign) (blocks, styles json.RawMessage, err error) {
	b, err := json.Marshal(c.Blocks)
	if err != nil {
		return nil, nil, err
	}
	s, err := json.Marshal(c.Styles)
	if err != nil {
		return nil, nil, err
	}
	return b, s, nil
}

func toCampaigns(rows []db.EmailCampaign) ([]Campaign, error) {
	out := make([]Campaign, 0, len(rows))
	for _, row := range rows {
		c, err := toCampaign(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func toCampaign(row db.EmailCampaign) (Campaign, error) {
	c := Campaign{
		ID:              row.ID,
		ChannelID:       row.ChannelID,
		Name:            row.Name,
		Subject:         row.Subject,
		AudienceKind:    row.AudienceKind,
		Status:          row.Status,
		TotalRecipients: int(row.TotalRecipients),
		SentCount:       int(row.SentCount),
		FailedCount:     int(row.FailedCount),
		SkippedCount:    int(row.SkippedCount),
		OpenedCount:     int(row.OpenedCount),
		ClickedCount:    int(row.ClickedCount),
		ABEnabled:       row.AbEnabled,
		ABSubjectB:      row.AbSubjectB,
		ABTestPercent:   int(row.AbTestPercent),
		ABWinnerMetric:  row.AbWinnerMetric,
		ABTestMinutes:   int(row.AbTestMinutes),
		ABWinner:        row.AbWinner,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		DeletedAt:       nullTimePtr(row.DeletedAt),
		MaterializedAt:  nullTimePtr(row.MaterializedAt),
	}
	if row.AbTestStartedAt.Valid {
		t := row.AbTestStartedAt.Time
		c.ABTestStartedAt = &t
	}
	if len(row.Blocks) > 0 {
		if err := json.Unmarshal(row.Blocks, &c.Blocks); err != nil {
			return Campaign{}, err
		}
	}
	if len(row.Styles) > 0 {
		if err := json.Unmarshal(row.Styles, &c.Styles); err != nil {
			return Campaign{}, err
		}
	}
	if row.AudienceID.Valid {
		id := row.AudienceID.UUID
		c.AudienceID = &id
	}
	if row.EmailID.Valid {
		id := row.EmailID.UUID
		c.EmailID = &id
	}
	if row.ScheduledFor.Valid {
		t := row.ScheduledFor.Time
		c.ScheduledFor = &t
	}
	if row.SentAt.Valid {
		t := row.SentAt.Time
		c.SentAt = &t
	}
	return c, nil
}

func toRecipient(row db.EmailCampaignRecipient) Recipient {
	r := Recipient{
		ID:         row.ID,
		CampaignID: row.CampaignID,
		Email:      row.Email,
		FirstName:  row.FirstName,
		Status:     row.Status,
		Error:      row.Error,
		Variant:    row.Variant,
		Phase:      row.Phase,
	}
	if row.CustomerID.Valid {
		id := row.CustomerID.UUID
		r.CustomerID = &id
	}
	return r
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

// nullTimePtr converts a nullable SQL timestamp into the *time.Time the
// domain structs use
func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func (r *repo) PruneSentRecipients(ctx context.Context, before time.Time) (int64, error) {
	ids, err := r.q.ListCampaignsWithRecipientsSentBefore(ctx, sql.NullTime{Time: before, Valid: true})
	if err != nil {
		return 0, err
	}
	var total int64
	for _, id := range ids {
		n, err := r.q.DeleteCampaignRecipients(ctx, id)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
