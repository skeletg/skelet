package customer

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

// Records what the direct sender was asked to send, standing in for *emailtemplate.Service
type recordingFallback struct {
	verifications, resets int
	lastFrom, lastTo      string
	lastLink, lastStore   string
}

func (r *recordingFallback) SendVerification(_ context.Context, _ uuid.UUID, from, to, storefrontURL string, d email.VerificationEmailData) error {
	r.verifications++
	r.lastFrom, r.lastTo, r.lastLink, r.lastStore = from, to, d.Link, storefrontURL
	return nil
}

func (r *recordingFallback) SendPasswordReset(_ context.Context, _ uuid.UUID, from, to, storefrontURL string, d email.PasswordResetEmailData) error {
	r.resets++
	r.lastFrom, r.lastTo, r.lastLink, r.lastStore = from, to, d.Link, storefrontURL
	return nil
}

// Reports n flows enrolled, so a test can stand in for both an active flow and none
type stubDispatcher struct{ n int }

func (s stubDispatcher) OnEmailVerification(context.Context, uuid.UUID, uuid.UUID, string, string, string) int {
	return s.n
}

func (s stubDispatcher) OnPasswordReset(context.Context, uuid.UUID, uuid.UUID, string, string, string, string) int {
	return s.n
}

// A paused, deleted or never-seeded flow must not swallow the mail that lets a customer back into their account
func TestAuthEmailFallbackSendsWhenNoFlowHandles(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dispatcher authEmailDispatcher
	}{
		{"no dispatcher at all", nil},
		{"dispatcher with no active flow", stubDispatcher{n: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			fb := &recordingFallback{}
			f.svc.SetAuthEmailDispatcher(tc.dispatcher)
			f.svc.SetAuthEmailFallback(fb)

			ch := f.newChannel(t)
			addr := randomEmail()
			c := f.register(t, ch, addr, "password123")
			if fb.verifications != 1 {
				t.Fatalf("verification sends = %d, want 1 from registration", fb.verifications)
			}
			if fb.lastTo != c.Email {
				t.Errorf("sent to %q, want %q", fb.lastTo, c.Email)
			}
			if !strings.Contains(fb.lastLink, "https://shop.invalid") {
				t.Errorf("verification link %q does not point at the store", fb.lastLink)
			}
			if fb.lastStore != "https://shop.invalid" {
				t.Errorf("storefront base = %q, want the bare origin the open pixel resolves against", fb.lastStore)
			}

			if err := f.svc.RequestPasswordReset(f.ctx, ch, addr); err != nil {
				t.Fatalf("RequestPasswordReset: %v", err)
			}
			if fb.resets != 1 {
				t.Errorf("reset sends = %d, want 1", fb.resets)
			}
		})
	}
}

// The flow engine owns delivery whenever a flow is active - sending both would double-mail every customer
func TestAuthEmailFallbackSkippedWhenFlowHandles(t *testing.T) {
	f := newFixture(t)
	fb := &recordingFallback{}
	f.svc.SetAuthEmailDispatcher(stubDispatcher{n: 1})
	f.svc.SetAuthEmailFallback(fb)

	ch := f.newChannel(t)
	addr := randomEmail()
	f.register(t, ch, addr, "password123")
	if err := f.svc.RequestPasswordReset(f.ctx, ch, addr); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if fb.verifications != 0 || fb.resets != 0 {
		t.Errorf("fallback sent %d verifications and %d resets, want none - a flow handled both", fb.verifications, fb.resets)
	}
}

// An unknown address must stay unobservable, fallback or not
func TestAuthEmailFallbackSilentForUnknownAddress(t *testing.T) {
	f := newFixture(t)
	fb := &recordingFallback{}
	f.svc.SetAuthEmailFallback(fb)

	ch := f.newChannel(t)
	if err := f.svc.RequestPasswordReset(f.ctx, ch, randomEmail()); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if fb.resets != 0 {
		t.Errorf("reset sends = %d for an address with no account, want 0", fb.resets)
	}
}
