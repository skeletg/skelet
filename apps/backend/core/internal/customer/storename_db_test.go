package customer

import (
	"context"
	"testing"

	"github.com/google/uuid"

	_ "github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

type namingFallback struct{ verifyName, resetName string }

func (n *namingFallback) SendVerification(_ context.Context, _ uuid.UUID, _, _, _ string, d email.VerificationEmailData) error {
	n.verifyName = d.StoreName
	return nil
}

func (n *namingFallback) SendPasswordReset(_ context.Context, _ uuid.UUID, _, _, _ string, d email.PasswordResetEmailData) error {
	n.resetName = d.StoreName
	return nil
}

func (n *namingFallback) SendSignupConfirmation(context.Context, uuid.UUID, string, string, string, email.SignupConfirmationEmailData) error {
	return nil
}

func (n *namingFallback) SendAccountExists(context.Context, uuid.UUID, string, string, string, email.AccountExistsEmailData) error {
	return nil
}

type namingDispatcher struct{ verifyName, resetName *string }

func (d namingDispatcher) OnEmailVerification(_ context.Context, _, _ uuid.UUID, _, storeName, _ string) int {
	*d.verifyName = storeName
	return 0
}

func (d namingDispatcher) OnPasswordReset(_ context.Context, _, _ uuid.UUID, _, _, storeName, _ string) int {
	*d.resetName = storeName
	return 0
}

func (d namingDispatcher) OnSignupConfirmation(context.Context, uuid.UUID, string, string, string, string, string) int {
	return 0
}

func (d namingDispatcher) OnAccountExists(context.Context, uuid.UUID, uuid.UUID, string, string, string, string) int {
	return 0
}

func TestAccountMailSignsWithTheCustomerFacingStoreName(t *testing.T) {
	f := newFixture(t)
	fb := &namingFallback{}
	var flowVerify, flowReset string
	f.svc.SetAuthEmailDispatcher(namingDispatcher{verifyName: &flowVerify, resetName: &flowReset})
	f.svc.SetAuthEmailFallback(fb)
	ch := f.newChannel(t)
	if _, err := f.cfg.Set(f.ctx, "general/branding/store_name", "Acme Outfitters", config.Scope{Kind: config.ScopeChannel, ChannelID: &ch}); err != nil {
		t.Fatal(err)
	}
	addr := randomEmail()
	f.register(t, ch, addr, "password123")
	if err := f.svc.RequestPasswordReset(f.ctx, ch, addr); err != nil {
		t.Fatal(err)
	}
	f.svc.FlushMail()
	for what, got := range map[string]string{"verification": fb.verifyName, "reset": fb.resetName, "verification flow": flowVerify, "reset flow": flowReset} {
		if got != "Acme Outfitters" {
			t.Errorf("%s mail signed as %q, want the channel's customer-facing store name", what, got)
		}
	}
}
