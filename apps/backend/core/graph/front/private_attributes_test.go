package front

import (
	"context"
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

type attributesDown struct{ catalog.AttributeRepository }

func (attributesDown) ListAttributes(context.Context) ([]catalog.Attribute, error) {
	return nil, errors.New("db down")
}

func TestAnUnreadableAttributeSetFailsClosed(t *testing.T) {
	r := &Resolver{CatalogSvc: catalog.NewService(catalog.Deps{Repo: catalog.ReposOf(attributesDown{})})}
	if private, known := r.privateAttributeSlugs(context.Background()); known || private != nil {
		t.Errorf("privateAttributeSlugs = %v, %v - want unknown so browse strips attribute filters and facets", private, known)
	}
}
