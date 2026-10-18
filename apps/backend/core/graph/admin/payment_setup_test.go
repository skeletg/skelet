package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
)

func TestPaymentProviderSetupRequiresAuthentication(t *testing.T) {
	r := &Resolver{}
	if _, err := (&queryResolver{r}).PaymentProviders(context.Background(), nil); err == nil {
		t.Fatal("anonymous provider metadata read allowed")
	}
	if _, err := (&mutationResolver{r}).UpdatePaymentMethodPresentation(context.Background(), uuid.NewString(), "cards", "Card", 1); err == nil {
		t.Fatal("anonymous presentation write allowed")
	}
}

func TestPaymentProviderSetupRespectsChannelScope(t *testing.T) {
	f := newEscalationFixture(t)
	allowed := f.channel
	other := uuid.NewString()
	_, ctx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "payments.manage", ChannelID: &allowed}))
	r := &Resolver{AdminSvc: f.svc}
	if _, err := (&queryResolver{r}).PaymentProviders(ctx, &other); !forbidden(err) {
		t.Fatalf("cross-channel metadata read = %v", err)
	}
	if _, err := (&mutationResolver{r}).UpdatePaymentMethodPresentation(ctx, other, "cards", "Card", 0); !forbidden(err) {
		t.Fatalf("cross-channel presentation write = %v", err)
	}
	allowedString := allowed.String()
	if _, err := (&queryResolver{r}).PaymentProviders(ctx, &allowedString); err != nil {
		t.Fatalf("allowed metadata read = %v", err)
	}
}

func TestPaymentProviderMetadataHasOnlyPathsAndChannelWebhookRouting(t *testing.T) {
	id := uuid.New()
	providers := paymentProviderModels(context.Background(), id)
	for _, provider := range providers {
		for _, path := range provider.ConfigurationPaths {
			if !strings.HasPrefix(path, "payment/"+provider.Slug+"/") && !strings.HasPrefix(path, "plugin/"+provider.PluginSlug+"/") {
				t.Fatalf("unrelated configuration path %q", path)
			}
		}
		if provider.IsOffline && provider.WebhookPath != nil {
			t.Fatalf("offline provider has webhook: %+v", provider)
		}
		if provider.WebhookPath != nil && !strings.HasSuffix(*provider.WebhookPath, "?channel="+id.String()) {
			t.Fatalf("webhook does not route channel: %s", *provider.WebhookPath)
		}
	}
}
