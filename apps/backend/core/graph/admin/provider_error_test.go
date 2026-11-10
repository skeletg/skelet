package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestAProviderErrorShowsAStoreAdminOnlyItsStatus(t *testing.T) {
	none := noPermissions{}
	msg := `ai: embeddings 400 from api.voyageai.com: {"detail":"account org-7f3 is over its quota"}`
	everyStore := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: true})
	if got := providerError(everyStore, none, msg); got != msg {
		t.Errorf("an admin over every store got %q - want the provider's own words", got)
	}
	storeOnly := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New()})
	if got := providerError(storeOnly, none, msg); got != "ai: embeddings 400 from api.voyageai.com" {
		t.Errorf("a store's admin got %q - want the status and host only", got)
	}
	if got := providerError(storeOnly, none, "ai: embeddings request to api.voyageai.com: dial tcp: i/o timeout"); got == "" || got == "ai: embeddings request to api.voyageai.com: dial tcp: i/o timeout" {
		t.Errorf("a store's admin got the raw transport error %q", got)
	}
}

type noPermissions struct{}

func (noPermissions) HasPermission(context.Context, uuid.UUID, string) (bool, error) {
	return false, nil
}
