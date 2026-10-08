package form

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type knownContact struct {
	id       uuid.UUID
	calls    int
	verified bool
}

func (c *knownContact) EnsureContact(_ context.Context, _ uuid.UUID, _, _ string, verified bool) (uuid.UUID, error) {
	c.calls++
	c.verified = verified
	return c.id, nil
}

type consentAdapter struct {
	svc *consent.Service
	err error
}

func (a *consentAdapter) SetConsent(ctx context.Context, id uuid.UUID, kind string, optedIn bool, source string) error {
	if a.err != nil {
		return a.err
	}
	_, err := a.svc.SetConsent(ctx, id, kind, optedIn, source)
	return err
}

type segmentLog struct{ assigned []uuid.UUID }

func (s *segmentLog) AssignCustomers(_ context.Context, _ uuid.UUID, ids []uuid.UUID) error {
	s.assigned = append(s.assigned, ids...)
	return nil
}

type flowLog struct{ fired int }

func (f *flowLog) OnFormSubmitted(context.Context, uuid.UUID, uuid.UUID, string) int {
	f.fired++
	return 1
}

type mailLog struct {
	to, tokens []string
}

func (m *mailLog) SendSignupConfirmation(_ context.Context, _ uuid.UUID, to, _, token, _ string) error {
	m.to, m.tokens = append(m.to, to), append(m.tokens, token)
	return nil
}

type budget struct{ allow bool }

func (b budget) Allow(context.Context, string) bool { return b.allow }

type confirmFixture struct {
	svc     *Service
	consent *consent.Service
	adapter *consentAdapter
	contact *knownContact
	segment *segmentLog
	flows   *flowLog
	mail    *mailLog
	conn    *sql.DB
	form    Form
	email   string
	ctx     context.Context
}

func newConfirmFixture(t *testing.T) *confirmFixture {
	t.Helper()
	conn := testdb.Require(t)
	q := db.New(conn)
	slug := "form-" + uuid.NewString()[:8]
	var ch, cust uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1 RETURNING id`, slug).Scan(&ch); err != nil {
		t.Skipf("no channel to clone currency and locale from: %v", err)
	}
	addr := slug + "@form.test"
	if err := conn.QueryRow(`INSERT INTO customers (channel_id, email, password_hash, customer_group_id)
		SELECT $1, $2, 'x', id FROM customer_groups ORDER BY is_default DESC LIMIT 1 RETURNING id`, ch, addr).Scan(&cust); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM signup_forms WHERE channel_id = $1`, ch)
		_, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, cust)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch)
	})

	f := &confirmFixture{
		consent: consent.NewService(consent.NewRepository(q)),
		contact: &knownContact{id: cust}, segment: &segmentLog{}, flows: &flowLog{}, mail: &mailLog{},
		conn: conn, email: addr, ctx: context.Background(),
	}
	f.adapter = &consentAdapter{svc: f.consent}
	f.svc = New(q)
	f.svc.SetHooks(f.contact, f.adapter, f.segment, f.flows)
	f.svc.SetConfirmation(f.mail, budget{allow: true})
	seg := uuid.New()
	form, err := f.svc.Create(f.ctx, SaveInput{
		ChannelID: ch, Slug: slug, Name: "Newsletter", Status: "active", TargetSegmentID: &seg, TriggerFlow: true,
	})
	if err != nil {
		t.Fatalf("create form: %v", err)
	}
	f.form = form
	return f
}

func (f *confirmFixture) optedIn(t *testing.T) bool {
	t.Helper()
	ok, err := f.consent.IsOptedIn(f.ctx, f.contact.id, consentkind.MarketingEmail)
	if err != nil {
		t.Fatalf("read consent: %v", err)
	}
	return ok
}

func (f *confirmFixture) submit(t *testing.T) string {
	t.Helper()
	before := len(f.mail.tokens)
	if _, err := f.svc.Submit(f.ctx, f.form.ChannelID, f.form.Slug, f.email, "Ana"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(f.mail.tokens) != before+1 {
		t.Fatalf("confirmation mails = %d, want one more", len(f.mail.tokens))
	}
	return f.mail.tokens[len(f.mail.tokens)-1]
}

func TestASubmitChangesNothingUntilTheAddressConfirms(t *testing.T) {
	f := newConfirmFixture(t)
	if _, err := f.consent.SetConsent(f.ctx, f.contact.id, consentkind.MarketingEmail, false, consent.SourceUnsubscribeLink); err != nil {
		t.Fatalf("seed opt-out: %v", err)
	}

	msg, err := f.svc.Submit(f.ctx, f.form.ChannelID, f.form.Slug, f.email, "Ana")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if msg != DefaultSuccessMessage {
		t.Errorf("message = %q, want the default that asks them to check their inbox", msg)
	}
	if len(f.mail.to) != 1 || f.mail.to[0] != f.email {
		t.Errorf("confirmation mailed to %v, want %s", f.mail.to, f.email)
	}
	if f.optedIn(t) {
		t.Error("an unconfirmed submit flipped the opt-out back on")
	}
	if f.contact.calls != 0 || len(f.segment.assigned) != 0 || f.flows.fired != 0 {
		t.Errorf("before confirmation: %d contact writes, %d segment adds, %d flows - want none", f.contact.calls, len(f.segment.assigned), f.flows.fired)
	}
	if n, _ := f.svc.SubmissionCount(f.ctx, f.form.ID); n != 1 {
		t.Errorf("leads = %d, want the submission captured on submit", n)
	}
}

func TestConfirmingSubscribesOnce(t *testing.T) {
	f := newConfirmFixture(t)
	token := f.submit(t)

	if ok, err := f.svc.ConfirmationPending(f.ctx, token); err != nil || !ok {
		t.Fatalf("pending = %v (%v), want the fresh link good", ok, err)
	}
	if err := f.svc.Confirm(f.ctx, token); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !f.optedIn(t) {
		t.Fatal("confirmation did not record the opt-in")
	}
	if !f.contact.verified {
		t.Error("the contact was ensured unverified, though the confirmation link proved the address")
	}
	hist, err := f.consent.History(f.ctx, f.contact.id, 10)
	if err != nil || len(hist) != 1 || hist[0].Source != consent.SourceSignupForm {
		t.Errorf("history = %+v (%v), want one signup_form opt-in", hist, err)
	}
	if len(f.segment.assigned) != 1 || f.segment.assigned[0] != f.contact.id || f.flows.fired != 1 {
		t.Errorf("segment adds %v, flows %d - want the subscriber added once and the welcome flow fired once", f.segment.assigned, f.flows.fired)
	}

	if err := f.svc.Confirm(f.ctx, token); !errors.Is(err, ErrConfirmationInvalid) {
		t.Errorf("second confirm = %v, want ErrConfirmationInvalid - the link is single-use", err)
	}
	if ok, _ := f.svc.ConfirmationPending(f.ctx, token); ok {
		t.Error("a used link still reads as pending")
	}
	if len(f.segment.assigned) != 1 || f.flows.fired != 1 {
		t.Error("a replayed link ran the segment add or the flow again")
	}
	if err := f.svc.Confirm(f.ctx, "not-a-token"); !errors.Is(err, ErrConfirmationInvalid) {
		t.Errorf("unknown token = %v, want ErrConfirmationInvalid", err)
	}
}

func TestAnExpiredLinkConfirmsNothing(t *testing.T) {
	f := newConfirmFixture(t)
	token := f.submit(t)
	if _, err := f.conn.Exec(`UPDATE signup_confirmations SET expires_at = now() - interval '1 second' WHERE token_hash = $1`, hashToken(token)); err != nil {
		t.Fatalf("expire link: %v", err)
	}
	if ok, _ := f.svc.ConfirmationPending(f.ctx, token); ok {
		t.Error("an expired link reads as pending")
	}
	if err := f.svc.Confirm(f.ctx, token); !errors.Is(err, ErrConfirmationInvalid) {
		t.Errorf("confirm = %v, want ErrConfirmationInvalid", err)
	}
	if f.optedIn(t) || f.contact.calls != 0 {
		t.Error("an expired link still subscribed the address")
	}
	n, err := f.svc.PruneConfirmations(f.ctx)
	if err != nil || n < 1 {
		t.Errorf("prune removed %d (%v), want the expired link gone", n, err)
	}
}

func TestAFailedConfirmationCanBeRetried(t *testing.T) {
	f := newConfirmFixture(t)
	token := f.submit(t)
	f.adapter.err = errors.New("db down")
	if err := f.svc.Confirm(f.ctx, token); err == nil || errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("confirm with consent failing = %v, want the underlying error", err)
	}
	f.adapter.err = nil
	if err := f.svc.Confirm(f.ctx, token); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !f.optedIn(t) {
		t.Error("the retried confirmation did not record the opt-in")
	}
}

func TestAnAddressOverItsBudgetGetsNoMoreMail(t *testing.T) {
	f := newConfirmFixture(t)
	f.svc.SetConfirmation(f.mail, budget{allow: false})
	msg, err := f.svc.Submit(f.ctx, f.form.ChannelID, f.form.Slug, f.email, "")
	if err != nil || msg != f.form.SuccessMessage {
		t.Fatalf("submit = %q, %v - want the same answer as any submit", msg, err)
	}
	if len(f.mail.to) != 0 {
		t.Errorf("mailed %d confirmations past the budget, want 0", len(f.mail.to))
	}
	var pending int
	if err := f.conn.QueryRow(`SELECT count(*) FROM signup_confirmations WHERE form_id = $1`, f.form.ID).Scan(&pending); err != nil || pending != 0 {
		t.Errorf("pending links = %d (%v), want none minted", pending, err)
	}
}

func TestPruneKeepsConfirmedLinks(t *testing.T) {
	f := newConfirmFixture(t)
	token := f.submit(t)
	if err := f.svc.Confirm(f.ctx, token); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	f.svc.now = func() time.Time { return time.Now().Add(2 * ConfirmationTTL) }
	if _, err := f.svc.PruneConfirmations(f.ctx); err != nil {
		t.Fatalf("prune: %v", err)
	}
	var left int
	if err := f.conn.QueryRow(`SELECT count(*) FROM signup_confirmations WHERE token_hash = $1 AND customer_id = $2`, hashToken(token), f.contact.id).Scan(&left); err != nil || left != 1 {
		t.Errorf("confirmed rows left = %d (%v), want the confirmed link kept and linked to the customer", left, err)
	}
}
