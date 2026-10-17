package front

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
)

type storedRedirect struct {
	urlrewrite.Repository
	row urlrewrite.UrlRewrite
}

func (s storedRedirect) GetByRequestPath(context.Context, string, *uuid.UUID) (urlrewrite.UrlRewrite, error) {
	return s.row, nil
}

func (s storedRedirect) ListByEntity(context.Context, urlrewrite.EntityType, uuid.UUID) ([]urlrewrite.UrlRewrite, error) {
	return []urlrewrite.UrlRewrite{{RequestPath: "live", RedirectType: urlrewrite.RedirectTypeCanonical}}, nil
}

// A row written before targets were validated must not send shoppers off-site
func TestAStoredOffSiteTargetIsNotFollowed(t *testing.T) {
	evil := "//evil.com"
	svc := urlrewrite.NewService(storedRedirect{row: urlrewrite.UrlRewrite{
		EntityType: urlrewrite.EntityTypeProduct, EntityID: uuid.New(), RequestPath: "old",
		RedirectType: urlrewrite.RedirectTypePermanent, TargetPath: &evil,
	}})
	res, err := (&queryResolver{&Resolver{URLRewriteSvc: svc}}).Route(context.Background(), "/old")
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.RedirectTo == nil || *res.RedirectTo != "/live" {
		t.Fatalf("redirect = %v, want the entity's live URL /live", res)
	}
}
