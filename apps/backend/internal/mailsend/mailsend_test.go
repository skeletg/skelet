package mailsend

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/email"
	"github.com/skeletg/skelet/apps/backend/internal/emailtrack"
)

type captureSender struct{ sent []email.Message }

func (c *captureSender) Send(_ context.Context, m email.Message) error {
	c.sent = append(c.sent, m)
	return nil
}

type stubLog struct {
	id     uuid.UUID
	err    error
	specs  []Spec
	called int
}

func (s *stubLog) RecordTransactional(_ context.Context, spec Spec) (uuid.UUID, error) {
	s.called++
	s.specs = append(s.specs, spec)
	return s.id, s.err
}

func newTracked(t *testing.T) (*Sender, *captureSender, *stubLog) {
	t.Helper()
	mail := &captureSender{}
	lg := &stubLog{id: uuid.New()}
	s := New(mail)
	s.SetTracking(lg, emailtrack.NewSigner([]byte("test-key")))
	return s, mail, lg
}

// The pixel references the logged send, so recording first is what stops it reporting as mail nobody read
func TestSendRecordsBeforeDeliveringAndCarriesThePixel(t *testing.T) {
	s, mail, lg := newTracked(t)
	spec := Spec{ChannelID: uuid.New(), From: "Shop <no-reply@shop.test>", To: "buyer@test", MessageKey: "order_confirmation"}

	if err := s.Send(context.Background(), spec, "https://shop.test/", "Receipt", "<html><body>hi</body></html>", "hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if lg.called != 1 {
		t.Fatalf("log calls = %d, want 1", lg.called)
	}
	if lg.specs[0].MessageKey != "order_confirmation" {
		t.Errorf("logged message key = %q, want the template slot", lg.specs[0].MessageKey)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("messages sent = %d, want 1", len(mail.sent))
	}
	body := mail.sent[0].HTMLBody
	if !strings.Contains(body, "https://shop.test/e/o/") {
		t.Errorf("no open pixel in the body:\n%s", body)
	}
	if strings.Index(body, "/e/o/") > strings.Index(body, "</body>") {
		t.Errorf("the pixel landed outside <body>, where clients may not load it:\n%s", body)
	}
	if !strings.Contains(body, "hi") {
		t.Errorf("the rendered content was lost:\n%s", body)
	}
	if mail.sent[0].From != spec.From || mail.sent[0].To != spec.To {
		t.Errorf("headers = %q -> %q, want %q -> %q", mail.sent[0].From, mail.sent[0].To, spec.From, spec.To)
	}
}

// Reporting is not worth a customer never getting their password-reset link
func TestSendDeliversEvenWhenTheLogFails(t *testing.T) {
	s, mail, lg := newTracked(t)
	lg.err = errors.New("signals down")

	if err := s.Send(context.Background(), Spec{To: "buyer@test"}, "https://shop.test", "Reset", "<html><body>x</body></html>", "x"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("messages sent = %d, want the mail delivered anyway", len(mail.sent))
	}
	if strings.Contains(mail.sent[0].HTMLBody, "/e/o/") {
		t.Error("a pixel was injected for a send that was never recorded, so it points at nothing")
	}
}

// Tracking is optional - an install with no recorder still sends
func TestSendWithoutTrackingStillDelivers(t *testing.T) {
	mail := &captureSender{}
	if err := New(mail).Send(context.Background(), Spec{To: "buyer@test"}, "https://shop.test", "Hi", "<html><body>x</body></html>", "x"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("messages sent = %d, want 1", len(mail.sent))
	}
	if strings.Contains(mail.sent[0].HTMLBody, "/e/o/") {
		t.Error("pixel injected with no log to reference")
	}
}
