package mailsend

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

type captureSender struct {
	sent []email.Message
	err  error
}

func (c *captureSender) Send(_ context.Context, m email.Message) error {
	if c.err != nil {
		return c.err
	}
	c.sent = append(c.sent, m)
	return nil
}

type stubRecorder struct {
	err    error
	specs  []Spec
	ids    []uuid.UUID
	called int
}

func (s *stubRecorder) RecordSent(_ context.Context, id uuid.UUID, spec Spec) error {
	s.called++
	s.ids = append(s.ids, id)
	s.specs = append(s.specs, spec)
	return s.err
}

func newRecorded(t *testing.T) (*Sender, *captureSender, *stubRecorder) {
	t.Helper()
	mail, rec := &captureSender{}, &stubRecorder{}
	s := New(mail, nil)
	s.SetRecorder(rec)
	return s, mail, rec
}

func TestSendDeliversAndReports(t *testing.T) {
	s, mail, rec := newRecorded(t)
	spec := Spec{ChannelID: uuid.New(), From: "Shop <no-reply@shop.test>", To: "buyer@test", MessageKey: "order_confirmation"}

	if err := s.Send(context.Background(), spec, "Receipt", "<p>hi</p>", "hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("messages sent = %d, want 1", len(mail.sent))
	}
	if mail.sent[0].From != spec.From || mail.sent[0].To != spec.To {
		t.Errorf("headers = %q -> %q, want %q -> %q", mail.sent[0].From, mail.sent[0].To, spec.From, spec.To)
	}
	s.Wait()
	if rec.called != 1 {
		t.Fatalf("recorder calls = %d, want 1", rec.called)
	}
	if rec.specs[0].MessageKey != "order_confirmation" {
		t.Errorf("reported message key = %q, want the template slot", rec.specs[0].MessageKey)
	}
	if rec.ids[0] == uuid.Nil {
		t.Error("reported send id is nil, so nothing downstream can correlate the open")
	}
}

// A mail that never left must not count as sent, or delivery rates read higher than reality
func TestAFailedDeliveryIsNotReported(t *testing.T) {
	s, mail, rec := newRecorded(t)
	mail.err = errors.New("smtp refused")

	if err := s.Send(context.Background(), Spec{To: "buyer@test"}, "Receipt", "<p>x</p>", "x"); err == nil {
		t.Fatal("Send returned nil for a refused delivery")
	}
	s.Wait()
	if rec.called != 0 {
		t.Errorf("recorder calls = %d, want the send left unreported", rec.called)
	}
}

// Reporting is not worth a customer never getting their password-reset link
func TestSendDeliversEvenWhenReportingFails(t *testing.T) {
	s, mail, rec := newRecorded(t)
	rec.err = errors.New("signals down")

	if err := s.Send(context.Background(), Spec{To: "buyer@test"}, "Reset", "<p>x</p>", "x"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("messages sent = %d, want the mail delivered anyway", len(mail.sent))
	}
}

type hungRecorder struct{ release chan struct{} }

func (h hungRecorder) RecordSent(ctx context.Context, _ uuid.UUID, _ Spec) error {
	select {
	case <-h.release:
	case <-ctx.Done():
	}
	return ctx.Err()
}

func TestSendDoesNotWaitOnAHungRecorder(t *testing.T) {
	s := New(&captureSender{}, nil)
	h := hungRecorder{release: make(chan struct{})}
	s.SetRecorder(h)
	done := make(chan error, 1)
	go func() { done <- s.Send(context.Background(), Spec{To: "buyer@test"}, "Reset", "<p>x</p>", "x") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Send blocked on the recorder")
	}
	close(h.release)
	s.Wait()
}

func TestSendWithoutARecorderStillDelivers(t *testing.T) {
	mail := &captureSender{}
	if err := New(mail, nil).Send(context.Background(), Spec{To: "buyer@test"}, "Hi", "<p>x</p>", "x"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("messages sent = %d, want 1", len(mail.sent))
	}
}

// The config service is optional in wiring, and a missing one must still produce a deliverable From
func TestFromWithoutConfig(t *testing.T) {
	s := New(&captureSender{}, nil)

	got := s.from(context.Background(), Spec{ChannelID: uuid.New(), FallbackName: "Vetrina"})
	if got != `"Vetrina" <no-reply@skelet.local>` {
		t.Errorf("From = %q, want the fallback name over the fallback address", got)
	}

	got = s.from(context.Background(), Spec{ChannelID: uuid.New()})
	if got != "no-reply@skelet.local" {
		t.Errorf("From = %q, want a bare address when nothing names the sender", got)
	}
	if strings.Contains(got, "<") {
		t.Errorf("From = %q, want no empty display-name brackets", got)
	}
}

// Account mail arrives through a projection that already resolved the identity, so Spec.From wins
func TestAResolvedFromIsNotSecondGuessed(t *testing.T) {
	s := New(&captureSender{}, nil)
	got := s.from(context.Background(), Spec{From: "Store <hi@store.test>", ChannelID: uuid.New(), FallbackName: "Ignored"})
	if got != "Store <hi@store.test>" {
		t.Errorf("From = %q, want the caller's own", got)
	}
}

// A sender name with a comma read as two addresses, and a non-ASCII one went out unencoded
func TestFromNamesAreQuotedAndEncoded(t *testing.T) {
	s := New(&captureSender{}, nil)
	for name, want := range map[string]string{
		"Shop, Inc":        `"Shop, Inc" <no-reply@skelet.local>`,
		"Prodavnica Čačak": "=?utf-8?q?Prodavnica_=C4=8Ca=C4=8Dak?= <no-reply@skelet.local>",
	} {
		got := s.from(context.Background(), Spec{FallbackName: name})
		if got != want {
			t.Errorf("From for %q = %q, want %q", name, got, want)
		}
		if a, err := mail.ParseAddress(got); err != nil || a.Name != name {
			t.Errorf("From %q doesn't parse back to one mailbox named %q: %v", got, name, err)
		}
	}
}

func TestLogsCarryTheDomainNotTheAddress(t *testing.T) {
	if got := recipientDomain("ana.markovic@example.test"); got != "*@example.test" {
		t.Errorf("recipientDomain = %q", got)
	}
	if got := recipientDomain("garbage"); got != "*" {
		t.Errorf("recipientDomain(no @) = %q", got)
	}
}
