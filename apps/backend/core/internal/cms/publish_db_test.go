package cms

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func newDBService(t *testing.T, requireApproval bool) (*Service, *events.Bus, *eventLog) {
	t.Helper()
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	bus := events.NewBus()
	log := &eventLog{}
	bus.SubscribeSync(EventPagePublishing, 0, log.record)
	bus.Subscribe(EventPagePublished, log.record)
	bus.Subscribe(EventPageUnpublished, log.record)
	svc := NewService(Deps{Repo: NewRepository(db.New(conn), tx), Tx: tx, Bus: bus, Config: fakeApprovalConfig(requireApproval)})
	return svc, bus, log
}

func seedPageType(t *testing.T, svc *Service) PageType {
	t.Helper()
	pt, err := svc.CreatePageType(context.Background(), CreatePageTypeInput{Slug: "t-" + uuid.NewString()[:8], Locale: "en", Name: "Test type"})
	if err != nil {
		t.Fatalf("seed page type: %v", err)
	}
	return pt
}

func seedPage(t *testing.T, svc *Service, pt PageType, slug string, parent *uuid.UUID) Page {
	t.Helper()
	p, err := svc.CreatePage(context.Background(), CreatePageInput{PageTypeID: pt.ID, ParentID: parent, Slug: slug, Locale: "en", Title: slug})
	if err != nil {
		t.Fatalf("seed page %q: %v", slug, err)
	}
	return p
}

func TestBulkSetPublishedIsAllOrNothingUnderTheGate(t *testing.T) {
	svc, bus, log := newDBService(t, true)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	approved := seedPage(t, svc, pt, "ok-"+uuid.NewString()[:8], nil)
	if _, err := svc.pageService.repo.SetPageWorkflowState(ctx, approved.ID, WorkflowApproved); err != nil {
		t.Fatalf("approve: %v", err)
	}
	draft := seedPage(t, svc, pt, "draft-"+uuid.NewString()[:8], nil)

	if _, err := svc.BulkSetPublished(ctx, []uuid.UUID{approved.ID, draft.ID}, true); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("err = %v, want ErrApprovalRequired", err)
	}
	waitIdle(t, bus)
	for _, id := range []uuid.UUID{approved.ID, draft.ID} {
		p, err := svc.GetPage(ctx, id, "en")
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if p.IsPublished {
			t.Errorf("page %s went live although the batch was refused", p.Slug)
		}
	}
	for _, e := range log.seen() {
		if e == EventPagePublished {
			t.Error("a rolled-back batch fired page.published")
		}
	}
}

func TestBulkSetPublishedSkipsLivePagesAndCountsChanges(t *testing.T) {
	svc, bus, log := newDBService(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	live := seedPage(t, svc, pt, "live-"+uuid.NewString()[:8], nil)
	if _, err := svc.PublishPage(ctx, live.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	waitIdle(t, bus)
	before := len(log.seen())
	fresh := seedPage(t, svc, pt, "fresh-"+uuid.NewString()[:8], nil)

	n, err := svc.BulkSetPublished(ctx, []uuid.UUID{live.ID, fresh.ID}, true)
	if err != nil {
		t.Fatalf("bulk publish: %v", err)
	}
	waitIdle(t, bus)
	if n != 1 {
		t.Errorf("changed %d, want 1 - the live page must be skipped", n)
	}
	published := 0
	for _, e := range log.seen()[before:] {
		if e == EventPagePublished {
			published++
		}
	}
	if published != 1 {
		t.Errorf("fired page.published %d times, want once for the page that changed", published)
	}
}

func TestUpdatePageSlugMovesEveryDescendantPath(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	root := "root-" + uuid.NewString()[:8]
	parent := seedPage(t, svc, pt, root, nil)
	child := seedPage(t, svc, pt, "child", &parent.ID)
	grandchild := seedPage(t, svc, pt, "grandchild", &child.ID)
	if grandchild.BasePath != root+"/child/grandchild" {
		t.Fatalf("seeded grandchild base_path = %q", grandchild.BasePath)
	}

	renamed := "renamed-" + uuid.NewString()[:8]
	if _, err := svc.UpdatePage(ctx, UpdatePageInput{ID: parent.ID, Slug: &renamed}); err != nil {
		t.Fatalf("rename: %v", err)
	}
	for id, want := range map[uuid.UUID]string{
		parent.ID:     renamed,
		child.ID:      renamed + "/child",
		grandchild.ID: renamed + "/child/grandchild",
	} {
		p, err := svc.GetPage(ctx, id, "en")
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if p.BasePath != want || p.URLPath != joinRoot(pt.RootSlug, want) {
			t.Errorf("page %s: base_path=%q url_path=%q, want %q", p.Slug, p.BasePath, p.URLPath, want)
		}
	}
}
