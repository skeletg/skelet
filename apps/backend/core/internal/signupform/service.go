// Package signupform powers storefront signup forms (popup / inline / flyout email capture) managed from the dashboard and feeding Pulse
// A submission records the lead and mails a double opt-in link - only following it creates the contact, records consent, adds the subscriber to the form's segment and fires the form_submitted flow
package signupform

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
)

// Dependency seams (accept interfaces) - set the customer/consent/segment/flow
// collaborators via SetHooks so the package stays cycle-free and testable
type contactEnsurer interface {
	EnsureContact(ctx context.Context, channelID uuid.UUID, email, firstName string, verified bool) (customerID uuid.UUID, err error)
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
type confirmationMailer interface {
	SendSignupConfirmation(ctx context.Context, channelID uuid.UUID, to, firstName, token, expiresIn string) error
}
type sendLimiter interface {
	Allow(ctx context.Context, key string) bool
}

const ConfirmationTTL = 7 * 24 * time.Hour

// What the confirmation mail says about the link's life, read off the TTL so the two can't disagree
var confirmationTTLText = fmt.Sprintf("%d days", int(ConfirmationTTL.Hours()/24))

// The longest first name a submission keeps - it lands in the confirmation mail and the contact
const maxNameRunes = 100

const defaultButtonLabel = "Subscribe"

// Service is the signup-form CRUD + submission API
type Service struct {
	q       *db.Queries
	contact contactEnsurer
	consent consentSetter
	segment segmentAssigner
	flows   flowEnroller
	mailer  confirmationMailer
	limiter sendLimiter
	now     func() time.Time
}

func New(q *db.Queries) *Service { return &Service{q: q, now: time.Now} }

// SetHooks wires the confirmation-time collaborators (optional; a bare service still
// does CRUD) - Called from main.go after the dependent services exist
func (s *Service) SetHooks(contact contactEnsurer, cons consentSetter, seg segmentAssigner, flows flowEnroller) {
	s.contact, s.consent, s.segment, s.flows = contact, cons, seg, flows
}

// SetConfirmation wires the double opt-in mail and the per-address budget on it - without a mailer a submission can't be confirmed, so Submit refuses
func (s *Service) SetConfirmation(m confirmationMailer, l sendLimiter) { s.mailer, s.limiter = m, l }

// DisplayConfig is the storefront widget's behaviour (stored as JSON)
type DisplayConfig struct {
	Trigger       string `json:"trigger"`       // immediate | delay | scroll | exit_intent
	DelaySeconds  int    `json:"delaySeconds"`  // for trigger=delay
	ScrollPercent int    `json:"scrollPercent"` // for trigger=scroll
	FrequencyDays int    `json:"frequencyDays"` // cookie cap: don't re-show for N days
}

// Holds the widget to values it can act on - an unknown trigger shows immediately, and a negative delay or a scroll past the page never fires
func (d DisplayConfig) sanitized() DisplayConfig {
	switch d.Trigger {
	case "immediate", "delay", "scroll", "exit_intent":
	default:
		d.Trigger = "immediate"
	}
	d.DelaySeconds = min(max(d.DelaySeconds, 0), 600)
	d.ScrollPercent = min(max(d.ScrollPercent, 0), 100)
	d.FrequencyDays = min(max(d.FrequencyDays, 0), 365)
	return d
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

// formConsentKind is the consent a confirmed signup records - one the shopper may decline, since the form only asks once they've confirmed
func formConsentKind(kind string) (string, error) {
	kind = orDefault(kind, consentkind.MarketingEmail)
	if def, ok := consentkind.Find(kind); !ok || def.Required {
		return "", ErrConsentKindInvalid
	}
	return kind, nil
}

func (s *Service) Create(ctx context.Context, in SaveInput) (Form, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Slug) == "" {
		return Form{}, ErrNameAndSlugRequired
	}
	kind, err := formConsentKind(in.ConsentKind)
	if err != nil {
		return Form{}, err
	}
	dc, _ := json.Marshal(in.Display.sanitized())
	row, err := s.q.CreateSignupForm(ctx, db.CreateSignupFormParams{
		ChannelID: in.ChannelID, Slug: slugify(in.Slug), Name: in.Name, Status: normStatus(in.Status),
		FormType: normType(in.Type), Heading: in.Heading, Body: in.Body, ButtonLabel: orDefault(in.ButtonLabel, defaultButtonLabel),
		SuccessMessage: orDefault(in.SuccessMessage, DefaultSuccessMessage), CollectName: in.CollectName,
		ConsentKind: kind, TargetSegmentID: sqlnull.UUIDNonZero(in.TargetSegmentID),
		TriggerFlow: in.TriggerFlow, DisplayConfig: dc,
	})
	if pgdb.IsUniqueViolation(err) {
		return Form{}, ErrSlugTaken
	}
	if err != nil {
		return Form{}, err
	}
	return toForm(row), nil
}

// The slug stays as created - the storefront and saved embeds address the form by it
func (s *Service) Update(ctx context.Context, id uuid.UUID, in SaveInput) (Form, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Form{}, ErrNameAndSlugRequired
	}
	kind, err := formConsentKind(in.ConsentKind)
	if err != nil {
		return Form{}, err
	}
	dc, _ := json.Marshal(in.Display.sanitized())
	row, err := s.q.UpdateSignupForm(ctx, db.UpdateSignupFormParams{
		ID: id, Name: in.Name, Status: normStatus(in.Status), FormType: normType(in.Type),
		Heading: in.Heading, Body: in.Body, ButtonLabel: orDefault(in.ButtonLabel, defaultButtonLabel),
		SuccessMessage: orDefault(in.SuccessMessage, DefaultSuccessMessage), CollectName: in.CollectName,
		ConsentKind: kind, TargetSegmentID: sqlnull.UUIDNonZero(in.TargetSegmentID),
		TriggerFlow: in.TriggerFlow, DisplayConfig: dc,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Form{}, ErrFormNotFound
	}
	if err != nil {
		return Form{}, err
	}
	return toForm(row), nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Form, error) {
	row, err := s.q.GetSignupForm(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Form{}, ErrFormNotFound
	}
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
	if limit <= 0 {
		limit = 100
	}
	limit = min(limit, 500)
	return s.q.ListSignupSubmissions(ctx, db.ListSignupSubmissionsParams{FormID: formID, Lim: limit})
}

// What a form says after a submit when the admin wrote nothing - it must not claim a subscription the confirmation hasn't made yet
const DefaultSuccessMessage = "Check your inbox to confirm your subscription."

// Submit records the lead and mails the double opt-in link - consent, segment and flow all wait for Confirm, so an address typed by someone else is never subscribed and an existing opt-out is never touched
func (s *Service) Submit(ctx context.Context, channelID uuid.UUID, formSlugOrID, email, name string) (string, error) {
	em := customer.NormalizeEmail(email)
	if em == "" {
		return "", ErrEmailRequired
	}
	// The address is mailed straight away, so it has to be one address and nothing else - the rule account creation uses
	if customer.ValidateEmail(em) != nil {
		return "", ErrEmailInvalid
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		return "", ErrNameTooLong
	}
	f, err := s.resolveActive(ctx, channelID, formSlugOrID)
	if err != nil {
		return "", err
	}
	if s.mailer == nil {
		return "", errors.New("signup confirmation mail is not wired")
	}
	if err := s.q.RecordSignupSubmission(ctx, db.RecordSignupSubmissionParams{
		FormID: f.ID, ChannelID: channelID, Email: em,
	}); err != nil {
		log.Printf("[form] record signup submission for %s: %v", f.Slug, err)
	}
	if s.limiter != nil && !s.limiter.Allow(ctx, "signup-confirm:"+channelID.String()+":"+em) {
		return f.SuccessMessage, nil
	}
	token, hash, err := newConfirmationToken()
	if err != nil {
		return "", err
	}
	first := strings.TrimSpace(name)
	if err := s.q.CreateSignupConfirmation(ctx, db.CreateSignupConfirmationParams{
		TokenHash: hash, FormID: f.ID, ChannelID: channelID, Email: em, FirstName: first,
		ConsentKind: orDefault(f.ConsentKind, consentkind.MarketingEmail), ExpiresAt: s.now().Add(ConfirmationTTL),
	}); err != nil {
		return "", fmt.Errorf("record signup confirmation: %w", err)
	}
	if err := s.mailer.SendSignupConfirmation(ctx, channelID, em, first, token, confirmationTTLText); err != nil {
		return "", fmt.Errorf("send signup confirmation: %w", err)
	}
	return f.SuccessMessage, nil
}

func (s *Service) ConfirmationPending(ctx context.Context, token string) (bool, error) {
	_, err := s.q.GetPendingSignupConfirmation(ctx, hashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// The claim is handed back when the contact or consent write fails, so the subscriber can press confirm again
func (s *Service) Confirm(ctx context.Context, token string) error {
	if s.contact == nil || s.consent == nil {
		return errors.New("signup confirmation hooks are not wired")
	}
	row, err := s.q.ClaimSignupConfirmation(ctx, hashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConfirmationInvalid
	}
	if err != nil {
		return fmt.Errorf("claim signup confirmation: %w", err)
	}
	custID, err := s.subscribe(ctx, row)
	if err != nil {
		if rerr := s.q.ReleaseSignupConfirmation(context.WithoutCancel(ctx), row.ID); rerr != nil {
			log.Printf("[form] release signup confirmation %s: %v", row.ID, rerr)
		}
		return err
	}
	if err := s.q.SetSignupConfirmationCustomer(ctx, db.SetSignupConfirmationCustomerParams{CustomerID: sqlnull.UUIDValueNonZero(custID), ID: row.ID}); err != nil {
		log.Printf("[form] link signup confirmation %s to customer %s: %v", row.ID, custID, err)
	}
	if err := s.q.RecordSignupSubmission(ctx, db.RecordSignupSubmissionParams{
		FormID: row.FormID, ChannelID: row.ChannelID, Email: row.Email, CustomerID: sqlnull.UUIDValueNonZero(custID),
	}); err != nil {
		log.Printf("[form] link signup submission to customer %s: %v", custID, err)
	}
	f, err := s.Get(ctx, row.FormID)
	if err != nil {
		log.Printf("[form] load form %s after confirmation: %v", row.FormID, err)
		return nil
	}
	if s.segment != nil && f.TargetSegmentID != nil {
		if err := s.segment.AssignCustomers(ctx, *f.TargetSegmentID, []uuid.UUID{custID}); err != nil {
			log.Printf("[form] assign customer %s to segment %s: %v", custID, *f.TargetSegmentID, err)
		}
	}
	if s.flows != nil && f.TriggerFlow {
		s.flows.OnFormSubmitted(ctx, row.ChannelID, custID, f.Slug)
	}
	return nil
}

func (s *Service) subscribe(ctx context.Context, row db.SignupConfirmation) (uuid.UUID, error) {
	// The confirmation link reached this inbox, so a contact made here starts verified
	custID, err := s.contact.EnsureContact(ctx, row.ChannelID, row.Email, row.FirstName, true)
	if err != nil {
		return uuid.Nil, fmt.Errorf("ensure contact: %w", err)
	}
	if err := s.consent.SetConsent(ctx, custID, row.ConsentKind, true, consent.SourceSignupForm); err != nil {
		return uuid.Nil, fmt.Errorf("record consent: %w", err)
	}
	return custID, nil
}

func (s *Service) PruneConfirmations(ctx context.Context) (int64, error) {
	return s.q.PruneSignupConfirmations(ctx, s.now())
}

// Only the hash is stored, so a read of the table can't confirm anyone
func newConfirmationToken() (token string, hash []byte, err error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", nil, fmt.Errorf("mint confirmation token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(buf[:])
	return token, hashToken(token), nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (s *Service) resolveActive(ctx context.Context, channelID uuid.UUID, slugOrID string) (Form, error) {
	if id, err := uuid.Parse(slugOrID); err == nil {
		f, err := s.Get(ctx, id)
		if err == nil && f.ChannelID == channelID && f.Status == "active" {
			return f, nil
		}
	}
	row, err := s.q.GetActiveSignupFormBySlug(ctx, db.GetActiveSignupFormBySlugParams{ChannelID: channelID, Slug: slugOrID})
	if errors.Is(err, sql.ErrNoRows) {
		return Form{}, ErrFormNotFound
	}
	if err != nil {
		return Form{}, err
	}
	return toForm(row), nil
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
