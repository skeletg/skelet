package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func TestPublishedContentIsNotReadableAnonymously(t *testing.T) {
	r := &queryResolver{&Resolver{}}
	ctx := context.Background()
	id := uuid.NewString()
	for name, call := range map[string]func() error{
		"blogPost":  func() error { _, err := r.BlogPost(ctx, nil, &id, nil); return err },
		"blogPosts": func() error { _, err := r.BlogPosts(ctx, nil, nil, nil); return err },
		"cmsPage":   func() error { _, err := r.CmsPage(ctx, nil, &id, nil); return err },
		"cmsPages":  func() error { _, err := r.CmsPages(ctx, nil, nil, nil); return err },
	} {
		if err := call(); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("%s answered an anonymous caller with %v, want unauthenticated - it serves private metadata and author emails", name, err)
		}
	}
}

func TestAMenuOnAnotherChannelCannotBeStrippedFromIt(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	mine, theirs := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{mine, theirs} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Menu','USD',true)`, ch, "menu-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	svc := menu.NewService(menu.NewRepository(db.New(conn)))
	m, err := svc.CreateMenu(ctx, menu.CreateMenuInput{Slug: "main-" + mine.String()[:8], Name: "Main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetMenuStores(ctx, m.ID, []uuid.UUID{mine, theirs}); err != nil {
		t.Fatal(err)
	}
	r := &mutationResolver{&Resolver{MenuSvc: svc}}
	scoped := adminWithCachedGrants(auth.Grant{Slug: "cms.manage", ChannelID: &mine})

	if _, err := r.SetMenuStores(scoped, m.ID.String(), []string{mine.String()}); err == nil {
		t.Fatal("a channel-scoped admin removed the menu from a channel they don't manage")
	}
	if _, err := r.UpdateMenu(scoped, menuUpdate(m.ID)); err == nil {
		t.Fatal("a channel-scoped admin renamed a menu another channel renders")
	}
	if got, _ := svc.ListMenuChannelAssignments(ctx, m.ID); len(got) != 2 {
		t.Fatalf("assignments = %v, want both channels kept", got)
	}

	own, err := svc.CreateMenu(ctx, menu.CreateMenuInput{Slug: "own-" + mine.String()[:8], Name: "Own"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetMenuStores(scoped, own.ID.String(), []string{mine.String()}); err != nil {
		t.Fatalf("assigning an unassigned menu to the admin's own channel: %v", err)
	}
	if _, err := r.SetMenuStores(scoped, own.ID.String(), []string{mine.String(), theirs.String()}); err == nil {
		t.Fatal("a channel-scoped admin assigned a menu to a channel they don't manage")
	}
}

func menuUpdate(id uuid.UUID) model.UpdateMenuInput {
	name := "Renamed"
	return model.UpdateMenuInput{ID: id.String(), Name: &name}
}

// The admin service reads grants through the request cache, so seeding it stands in for the database
func adminWithCachedGrants(grants ...auth.Grant) context.Context {
	id := uuid.New()
	ctx := auth.WithRequestCache(auth.WithAdmin(context.Background(), &auth.Claims{AdminID: id, Email: "a@b.c"}))
	_, _ = auth.RequestCacheFrom(ctx).GetOrLoad("admin-grants:"+id.String(), func() (any, error) { return auth.NewGrantSet(grants), nil })
	return auth.WithGrantChecker(ctx, auth.NewGrantChecker(id, auth.NewGrantSet(grants)))
}
