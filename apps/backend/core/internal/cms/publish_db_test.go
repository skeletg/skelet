package cms

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func newDBService(t *testing.T, requireApproval bool) (*Service, *events.Bus, *eventLog) {
	t.Helper()
	svc, bus, log, _ := newDBServiceConn(t, requireApproval)
	return svc, bus, log
}

func newDBServiceConn(t *testing.T, requireApproval bool) (*Service, *events.Bus, *eventLog, *sql.DB) {
	t.Helper()
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	bus := events.NewBus()
	log := &eventLog{}
	bus.SubscribeSync(EventPagePublishing, 0, log.record)
	bus.Subscribe(EventPagePublished, log.record)
	bus.Subscribe(EventPageUnpublished, log.record)
	svc := NewService(Deps{Repo: NewRepository(db.New(conn), tx), Tx: tx, Bus: bus, Config: fakeApprovalConfig(requireApproval)})
	return svc, bus, log, conn
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
	if _, err := svc.pageWorkflowService.repo.SetPageWorkflowState(ctx, approved.ID, WorkflowApproved); err != nil {
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

// A hook touching an earlier page in the batch must not wait on the batch's own row lock
func TestBulkSetPublishedHooksRunBeforeTheBatchLocksAnything(t *testing.T) {
	svc, _, _, conn := newDBServiceConn(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	first := seedPage(t, svc, pt, "first-"+uuid.NewString()[:8], nil)
	second := seedPage(t, svc, pt, "second-"+uuid.NewString()[:8], nil)
	svc.bus.SubscribeSync(EventPagePublishing, 1, func(ctx context.Context, e events.Event) error {
		if e.(*PagePublishing).PageID != second.ID {
			return nil
		}
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_, err := conn.ExecContext(ctx, `UPDATE cms_pages SET updated_at = NOW() WHERE id = $1`, first.ID)
		return err
	})

	n, err := svc.BulkSetPublished(ctx, []uuid.UUID{first.ID, second.ID}, true)
	if err != nil {
		t.Fatalf("bulk publish: %v - the hook blocked on a lock the batch was holding", err)
	}
	if n != 2 {
		t.Errorf("changed %d, want 2", n)
	}
}

func TestPromoteDuePagesGoesLiveInOneWrite(t *testing.T) {
	svc, _, _ := newDBService(t, true)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	p := seedPage(t, svc, pt, "sched-"+uuid.NewString()[:8], nil)
	at := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := svc.pageService.repo.SetScheduledAt(ctx, p.ID, &at); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if _, err := svc.pageWorkflowService.repo.SetPageWorkflowState(ctx, p.ID, WorkflowApproved); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if _, err := svc.PromoteDuePages(ctx); err != nil {
		t.Fatalf("promote: %v", err)
	}
	got, err := svc.GetPage(ctx, p.ID, "en")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.IsPublished || got.WorkflowState != WorkflowPublished || got.ScheduledAt != nil {
		t.Errorf("published=%v state=%s scheduled=%v, want live, published and unscheduled", got.IsPublished, got.WorkflowState, got.ScheduledAt)
	}
	if got.PublishedAt == nil || !got.PublishedAt.Equal(at) {
		t.Errorf("published_at = %v, want the scheduled time %v", got.PublishedAt, at)
	}
}

func TestBulkSetPublishedCountsARepeatedIDOnce(t *testing.T) {
	svc, bus, log := newDBService(t, false)
	ctx := context.Background()
	p := seedPage(t, svc, seedPageType(t, svc), "twice-"+uuid.NewString()[:8], nil)
	n, err := svc.BulkSetPublished(ctx, []uuid.UUID{p.ID, p.ID}, true)
	if err != nil {
		t.Fatalf("bulk publish: %v", err)
	}
	waitIdle(t, bus)
	published := 0
	for _, e := range log.seen() {
		if e == EventPagePublished {
			published++
		}
	}
	if n != 1 || published != 1 {
		t.Errorf("changed %d and fired page.published %d times, want 1 and 1", n, published)
	}
}
