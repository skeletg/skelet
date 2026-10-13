package front

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestAStorelessRequestCannotReachAnUnlistedPage(t *testing.T) {
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	svc := cms.NewService(cms.Deps{Repo: cms.NewRepository(db.New(conn), tx), Tx: tx})
	ctx := context.Background()
	sfx := uuid.NewString()[:8]
	pt, err := svc.CreatePageType(ctx, cms.CreatePageTypeInput{Slug: "vis-" + sfx, Locale: "en", Name: "Vis"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreatePage(ctx, cms.CreatePageInput{PageTypeID: pt.ID, Slug: "summer-" + sfx, Locale: "en", Title: "Summer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishPage(ctx, p.ID); err != nil {
		t.Fatal(err)
	}

	q := &queryResolver{&Resolver{CMSSvc: svc}}
	id := p.ID.String()
	for name, args := range map[string][3]*string{"slug": {&p.Slug, nil, nil}, "id": {nil, &id, nil}, "urlPath": {nil, nil, &p.URLPath}} {
		got, err := q.CmsPage(ctx, args[0], args[1], args[2])
		if got != nil {
			t.Errorf("cmsPage by %s served the unlisted page (err %v)", name, err)
		}
	}
	list, err := q.CmsPages(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		if m.ID == id {
			t.Error("cmsPages listed the unlisted page")
		}
	}
}
