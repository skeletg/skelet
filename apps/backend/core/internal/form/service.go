// Package form powers storefront signup forms (popup / inline / flyout email capture) managed from the dashboard and feeding Pulse - A submission ensures a marketing contact exists for the email (one profile per address), records consent, optionally adds the subscriber to a segment, and fires the form_submitted flow trigger for a welcome series
package form

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

// Dependency seams (accept interfaces) - set the customer/consent/segment/flow
// collaborators via SetHooks so the package stays cycle-free and testable
type contactEnsurer interface {
	EnsureContact(ctx context.Context, channelID uuid.UUID, email, firstName string) (customerID uuid.UUID, err error)
}
type consentSetter interface {
	SetConsent(ctx context.Context, customerID uuid.UUID, kind string, optedIn bool, source string) error
}
type segmentAssigner interface {
	AssignCustomers(ctx context.Context, segmentID uuid.UUID, ids []uuid.UUID) error
}
type flowEnroller interface {
	OnFormSubmitted(ctx context.Context, channelID, customerID uuid.UUID, formSlug string) int
}

// Service is the signup-form CRUD + submission API
type Service struct {
	q       *db.Queries
	contact contactEnsurer
	consent consentSetter
	segment segmentAssigner
	flows   flowEnroller
}

func New(q *db.Queries) *Service { return &Service{q: q} }

// SetHooks wires the submit-time collaborators (optional; a bare service still
// does CRUD) - Called from main.go after the dependent services exist
func (s *Service) SetHooks(contact contactEnsurer, cons consentSetter, seg segmentAssigner, flows flowEnroller) {
	s.contact, s.consent, s.segment, s.flows = contact, cons, seg, flows
}

// DisplayConfig is the storefront widget's behaviour (stored as JSON)
type DisplayConfig struct {
	Trigger       string `json:"trigger"`       // immediate | delay | scroll | exit_intent
	DelaySeconds  int    `json:"delaySeconds"`  // for trigger=delay
	ScrollPercent int    `json:"scrollPercent"` // for trigger=scroll
	FrequencyDays int    `json:"frequencyDays"` // cookie cap: don't re-show for N days
}

type Form struct {
	ID              uuid.UUID
	ChannelID       uuid.UUID
	Slug            string
	Name            string
	Status          string
	Type            string
	Heading         string
	Body            string
	ButtonLabel     string
	SuccessMessage  string
	CollectName     bool
	ConsentKind     string
	TargetSegmentID *uuid.UUID
	TriggerFlow     bool
	Display         DisplayConfig
}

// SaveInput captures the admin-mutable fields
type SaveInput struct {
	ChannelID       uuid.UUID
	Slug            string
	Name            string
	Status          string
	Type            string
	Heading         string
	Body            string
	ButtonLabel     string
	SuccessMessage  string
	CollectName     bool
	ConsentKind     string
	TargetSegmentID *uuid.UUID
	TriggerFlow     bool
	Display         DisplayConfig
}

func toForm(r db.SignupForm) Form {
	var dc DisplayConfig
	_ = json.Unmarshal(r.DisplayConfig, &dc)
	f := Form{
		ID: r.ID, ChannelID: r.ChannelID, Slug: r.Slug, Name: r.Name, Status: r.Status, Type: r.FormType,
		Heading: r.Heading, Body: r.Body, ButtonLabel: r.ButtonLabel, SuccessMessage: r.SuccessMessage,
		CollectName: r.CollectName, ConsentKind: r.ConsentKind, TriggerFlow: r.TriggerFlow, Display: dc,
	}
	if r.TargetSegmentID.Valid {
		id := r.TargetSegmentID.UUID
		f.TargetSegmentID = &id
	}
	return f
}

func normStatus(s string) string {
	if s == "active" {
		return "active"
	}
	return "draft"
}
func normType(t string) string {
	switch t {
	case "inline", "flyout", "popup":
		return t
	}
	return "popup"
}

func (s *Service) Create(ctx context.Context, in SaveInput) (Form, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" {
		return Form{}, fmt.Errorf("name and slug are required")
	}
	dc, _ := json.Marshal(in.Display)
	row, err := s.q.CreateSignupForm(ctx, db.CreateSignupFormParams{
		ChannelID: in.ChannelID, Slug: slugify(in.Slug), Name: in.Name, Status: normStatus(in.Status),
		FormType: normType(in.Type), Heading: in.Heading, Body: in.Body, ButtonLabel: orDefault(in.ButtonLabel, "Subscribe"),
		SuccessMessage: orDefault(in.SuccessMessage, "Thanks for subscribing!"), CollectName: in.CollectName,
		ConsentKind: orDefault(in.ConsentKind, "marketing_email"), TargetSegmentID: nullUUID(in.TargetSegmentID),
		TriggerFlow: in.TriggerFlow, DisplayConfig: dc,
	})
	if err != nil {
		return Form{}, err
	}
	return toForm(row), nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in SaveInput) (Form, error) {
	dc, _ := json.Marshal(in.Display)
	row, err := s.q.UpdateSignupForm(ctx, db.UpdateSignupFormParams{
		ID: id, Name: in.Name, Status: normStatus(in.Status), FormType: normType(in.Type),
		Heading: in.Heading, Body: in.Body, ButtonLabel: orDefault(in.ButtonLabel, "Subscribe"),
		SuccessMessage: orDefault(in.SuccessMessage, "Thanks for subscribing!"), CollectName: in.CollectName,
		ConsentKind: orDefault(in.ConsentKind, "marketing_email"), TargetSegmentID: nullUUID(in.TargetSegmentID),
		TriggerFlow: in.TriggerFlow, DisplayConfig: dc,
	})
	if err != nil {
		return Form{}, err
	}
	return toForm(row), nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Form, error) {
	row, err := s.q.GetSignupForm(ctx, id)
	if err != nil {
		return Form{}, err
	}
	return toForm(row), nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteSignupForm(ctx, id)
}

func (s *Service) List(ctx context.Context, channelID uuid.UUID) ([]Form, error) {
	rows, err := s.q.ListSignupForms(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return mapForms(rows), nil
}

// Active returns the channel's active forms - the storefront widget's feed
func (s *Service) Active(ctx context.Context, channelID uuid.UUID) ([]Form, error) {
	rows, err := s.q.ListActiveSignupForms(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return mapForms(rows), nil
}

// SubmissionCount / recent submissions back the dashboard reporting
func (s *Service) SubmissionCount(ctx context.Context, formID uuid.UUID) (int64, error) {
	return s.q.CountSignupSubmissions(ctx, formID)
}

func (s *Service) RecentSubmissions(ctx context.Context, formID uuid.UUID, limit int32) ([]db.ListSignupSubmissionsRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.q.ListSignupSubmissions(ctx, db.ListSignupSubmissionsParams{FormID: formID, Lim: limit})
}

// Submit processes a storefront submission for an active form: capture the lead, ensure a marketing contact, record consent, add to the target segment, and fire the welcome flow - Best-effort past the capture - a downstream hiccup (segment add, flow) must not fail the shopper's submit - Returns the form's success message. name may be ""
func (s *Service) Submit(ctx context.Context, channelID uuid.UUID, formSlugOrID, email, name string) (string, error) {
	em := strings.ToLower(strings.TrimSpace(email))
	if em == "" {
		return "", ErrEmailRequired
	}
	f, err := s.resolveActive(ctx, channelID, formSlugOrID)
	if err != nil {
		return "", err
	}

	// Ensure a contact so consent/segment/flow all key on a customer id
	var custID uuid.UUID
	if s.contact != nil {
		if id, cerr := s.contact.EnsureContact(ctx, channelID, em, name); cerr == nil {
			custID = id
		}
	}

	if err := s.q.RecordSignupSubmission(ctx, db.RecordSignupSubmissionParams{
		FormID: f.ID, ChannelID: channelID, Email: em, CustomerID: nullUUIDv(custID),
	}); err != nil {
		log.Printf("[form] record signup submission for %s: %v", f.Slug, err)
	}

	if custID != uuid.Nil {
		if s.consent != nil {
			// The consent record is the legal basis for mailing this person - dropping it silently means the
			// opt-in they gave isn't on file, and the next audit or unsubscribe dispute has nothing to show
			if err := s.consent.SetConsent(ctx, custID, orDefault(f.ConsentKind, "marketing_email"), true, consent.SourceRegistration); err != nil {
				log.Printf("[form] record consent for customer %s: %v", custID, err)
			}
		}
		if s.segment != nil && f.TargetSegmentID != nil {
			if err := s.segment.AssignCustomers(ctx, *f.TargetSegmentID, []uuid.UUID{custID}); err != nil {
				log.Printf("[form] assign customer %s to segment %s: %v", custID, *f.TargetSegmentID, err)
			}
		}
		if s.flows != nil && f.TriggerFlow {
			s.flows.OnFormSubmitted(ctx, channelID, custID, f.Slug)
		}
	}
	return f.SuccessMessage, nil
}

func (s *Service) resolveActive(ctx context.Context, channelID uuid.UUID, slugOrID string) (Form, error) {
	if id, err := uuid.Parse(slugOrID); err == nil {
		f, err := s.Get(ctx, id)
		if err == nil && f.ChannelID == channelID && f.Status == "active" {
			return f, nil
		}
	}
	active, err := s.Active(ctx, channelID)
	if err != nil {
		return Form{}, err
	}
	for _, f := range active {
		if f.Slug == slugOrID {
			return f, nil
		}
	}
	return Form{}, ErrFormNotFound
}

func mapForms(rows []db.SignupForm) []Form {
	out := make([]Form, 0, len(rows))
	for _, r := range rows {
		out = append(out, toForm(r))
	}
	return out
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func nullUUID(p *uuid.UUID) uuid.NullUUID {
	if p == nil || *p == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

func nullUUIDv(id uuid.UUID) uuid.NullUUID {
	if id == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: id, Valid: true}
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "form"
	}
	return out
}
