package cms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/blockkit"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type urlRig struct {
	svc  *Service
	conn *sql.DB
	bus  *events.Bus
	log  *eventLog
}

func newURLRig(t *testing.T) urlRig {
	t.Helper()
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	bus := events.NewBus()
	log := &eventLog{}
	bus.SubscribeSync(EventPagePublishing, 0, log.record)
	for _, e := range []string{EventPagePublished, EventPageUnpublished, EventPageChanged} {
		bus.Subscribe(e, log.record)
	}
	rw := urlrewrite.NewService(urlrewrite.NewRepository(db.New(conn)))
	svc := NewService(Deps{Repo: NewRepository(db.New(conn), tx), Tx: tx, Bus: bus, URLRewrites: rw, Config: fakeApprovalConfig(false)})
	return urlRig{svc: svc, conn: conn, bus: bus, log: log}
}

func sfx() string { return uuid.NewString()[:8] }

func (r urlRig) canonical(t *testing.T, pageID uuid.UUID) string {
	t.Helper()
	var path string
	err := r.conn.QueryRow(`SELECT request_path FROM url_rewrites WHERE entity_type = 'cms_page' AND entity_id = $1 AND redirect_type = 0 AND channel_id IS NULL`, pageID).Scan(&path)
	if err != nil {
		t.Fatalf("canonical for %s: %v", pageID, err)
	}
	return path
}

func (r urlRig) redirects(t *testing.T, pageID uuid.UUID) map[string]bool {
	t.Helper()
	rows, err := r.conn.Query(`SELECT request_path FROM url_rewrites WHERE entity_type = 'cms_page' AND entity_id = $1 AND redirect_type = 301`, pageID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		out[p] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func rooted(t *testing.T, svc *Service, root string) PageType {
	t.Helper()
	pt := seedPageType(t, svc)
	pt, err := svc.UpdatePageTypeStructure(context.Background(), UpdatePageTypeStructureInput{ID: pt.ID, RootSlug: &root})
	if err != nil {
		t.Fatalf("root %q: %v", root, err)
	}
	return pt
}

func TestCreateAndCloneWritePageValuesInsideTheCreateTx(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	color, err := svc.CreateAttribute(ctx, CreateAttributeInput{Slug: "color-" + sfx(), InputType: InputSelect, Values: []AttributeValueInput{{Slug: "red"}, {Slug: "blue", Position: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	target := TargetCMSPage
	related, err := svc.CreateAttribute(ctx, CreateAttributeInput{Slug: "related-" + sfx(), InputType: InputRelationship, RelationshipTarget: &target})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []Attribute{color, related} {
		if _, err := svc.AssignAttribute(ctx, AssignAttributeInput{PageTypeID: pt.ID, AttributeID: a.ID}); err != nil {
			t.Fatal(err)
		}
	}
	other := seedPage(t, svc, pt, "other-"+sfx(), nil)

	p, err := svc.CreatePage(ctx, CreatePageInput{
		PageTypeID: pt.ID, Slug: "valued-" + sfx(), Locale: "en", Title: "Valued",
		Selections:    []PageAttributeSelection{{AttributeID: color.ID, ValueID: color.Values[1].ID}},
		Relationships: []PageRelationship{{AttributeID: related.ID, TargetType: TargetCMSPage, TargetID: other.ID}},
	})
	if err != nil {
		t.Fatalf("create with selections and relationships: %v", err)
	}
	if len(p.Selections) != 1 || len(p.Relationships) != 1 {
		t.Fatalf("created page carries %d selections and %d relationships, want 1 and 1", len(p.Selections), len(p.Relationships))
	}
	clone, err := svc.ClonePage(ctx, p.ID)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if len(clone.Selections) != 1 || clone.Selections[0].ValueID != color.Values[1].ID || len(clone.Relationships) != 1 || clone.Relationships[0].TargetID != other.ID {
		t.Errorf("clone selections %+v relationships %+v, want the source's", clone.Selections, clone.Relationships)
	}
}

func TestPageValuesAreCheckedAgainstTheType(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	size, err := svc.CreateAttribute(ctx, CreateAttributeInput{Slug: "size-" + sfx(), InputType: InputSelect, Values: []AttributeValueInput{{Slug: "s"}, {Slug: "m", Position: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	stray, err := svc.CreateAttribute(ctx, CreateAttributeInput{Slug: "stray-" + sfx(), InputType: InputSelect, Values: []AttributeValueInput{{Slug: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	target := TargetCMSPage
	related, err := svc.CreateAttribute(ctx, CreateAttributeInput{Slug: "rel-" + sfx(), InputType: InputRelationship, RelationshipTarget: &target})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []Attribute{size, related} {
		if _, err := svc.AssignAttribute(ctx, AssignAttributeInput{PageTypeID: pt.ID, AttributeID: a.ID, IsRequired: a.ID == size.ID}); err != nil {
			t.Fatal(err)
		}
	}
	p := seedPage(t, svc, pt, "checked-"+sfx(), nil)

	for name, fn := range map[string]func() error{
		"a value of another attribute": func() error {
			_, err := svc.SetPageSelections(ctx, p.ID, size.ID, []uuid.UUID{stray.Values[0].ID})
			return err
		},
		"an attribute the type lacks": func() error {
			_, err := svc.SetPageSelections(ctx, p.ID, stray.ID, []uuid.UUID{stray.Values[0].ID})
			return err
		},
		"two values on a single select": func() error {
			_, err := svc.SetPageSelections(ctx, p.ID, size.ID, []uuid.UUID{size.Values[0].ID, size.Values[1].ID})
			return err
		},
		"a target that doesn't exist": func() error {
			_, err := svc.SetPageRelationships(ctx, p.ID, related.ID, []PageRelationship{{TargetType: TargetCMSPage, TargetID: uuid.New()}})
			return err
		},
		"a required select left empty on create": func() error {
			_, err := svc.CreatePage(ctx, CreatePageInput{PageTypeID: pt.ID, Slug: "req-" + sfx(), Locale: "en", Title: "x", Data: json.RawMessage(`{}`), ContentData: json.RawMessage(`{}`)})
			return err
		},
	} {
		err := fn()
		if !errors.Is(err, ErrInvalidFieldValue) && !errors.Is(err, ErrRequiredFieldMissing) {
			t.Errorf("%s: err = %v, want a field validation error", name, err)
		}
	}
	if _, err := svc.SetPageSelections(ctx, p.ID, size.ID, []uuid.UUID{size.Values[1].ID}); err != nil {
		t.Errorf("a valid selection was refused: %v", err)
	}
}

func TestUnassigningAnAttributeClearsItsPageValues(t *testing.T) {
	svc, _, _, conn := newDBServiceConn(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	a, err := svc.CreateAttribute(ctx, CreateAttributeInput{Slug: "un-" + sfx(), InputType: InputMultiSelect, Values: []AttributeValueInput{{Slug: "a"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AssignAttribute(ctx, AssignAttributeInput{PageTypeID: pt.ID, AttributeID: a.ID}); err != nil {
		t.Fatal(err)
	}
	p := seedPage(t, svc, pt, "un-"+sfx(), nil)
	if _, err := svc.SetPageSelections(ctx, p.ID, a.ID, []uuid.UUID{a.Values[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteAttributeValue(ctx, a.Values[0].ID); !errors.Is(err, ErrAttributeValueInUse) {
		t.Errorf("deleting a selected value = %v, want ErrAttributeValueInUse", err)
	}
	if _, err := svc.UnassignAttribute(ctx, pt.ID, a.ID); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM cms_page_attribute_values WHERE page_id = $1`, p.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d page values outlived the unassignment", n)
	}
	if err := svc.DeleteAttribute(ctx, a.ID); err != nil {
		t.Errorf("deleting the now-unused attribute: %v", err)
	}
}

func TestARootChangeThatClashesMovesNothingAndARetryMovesEverything(t *testing.T) {
	rig := newURLRig(t)
	ctx := context.Background()
	s := sfx()
	pt := rooted(t, rig.svc, "docs-"+s)
	a := seedPage(t, rig.svc, pt, "a-"+s, nil)
	b := seedPage(t, rig.svc, pt, "b-"+s, nil)
	newRoot := "kb-" + s
	blockerType := rooted(t, rig.svc, newRoot)
	blocker := seedPage(t, rig.svc, blockerType, "blk-"+s, nil)
	if _, err := rig.conn.Exec(`UPDATE url_rewrites SET request_path = $2 WHERE entity_id = $1 AND redirect_type = 0`, blocker.ID, newRoot+"/b-"+s); err != nil {
		t.Fatal(err)
	}

	if _, err := rig.svc.UpdatePageTypeStructure(ctx, UpdatePageTypeStructureInput{ID: pt.ID, RootSlug: &newRoot}); !errors.Is(err, ErrURLPathConflict) {
		t.Fatalf("clashing root = %v, want ErrURLPathConflict", err)
	}
	got, err := rig.svc.GetPageType(ctx, pt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RootSlug != "docs-"+s {
		t.Errorf("root = %q although the move was refused", got.RootSlug)
	}
	for _, p := range []Page{a, b} {
		if c := rig.canonical(t, p.ID); c != "docs-"+s+"/"+p.Slug {
			t.Errorf("page %s canonical moved to %q although the move was refused", p.Slug, c)
		}
	}

	if err := rig.svc.DeletePage(ctx, blocker.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.svc.UpdatePageTypeStructure(ctx, UpdatePageTypeStructureInput{ID: pt.ID, RootSlug: &newRoot}); err != nil {
		t.Fatalf("retry: %v", err)
	}
	for _, p := range []Page{a, b} {
		want := newRoot + "/" + p.Slug
		if c := rig.canonical(t, p.ID); c != want {
			t.Errorf("page %s canonical = %q, want %q", p.Slug, c, want)
		}
		got, err := rig.svc.GetPage(ctx, p.ID, "en")
		if err != nil {
			t.Fatal(err)
		}
		if got.URLPath != want {
			t.Errorf("page %s url_path = %q, want %q", p.Slug, got.URLPath, want)
		}
		if !rig.redirects(t, p.ID)["docs-"+s+"/"+p.Slug] {
			t.Errorf("page %s left no 301 from its old path", p.Slug)
		}
	}
}

func TestAnyStructureSaveRepairsAPageWhoseURLNeverMoved(t *testing.T) {
	rig := newURLRig(t)
	ctx := context.Background()
	s := sfx()
	pt := rooted(t, rig.svc, "help-"+s)
	p := seedPage(t, rig.svc, pt, "stuck-"+s, nil)
	if _, err := rig.conn.Exec(`UPDATE cms_pages SET url_path = $2 WHERE id = $1`, p.ID, "old-"+s+"/"+p.Slug); err != nil {
		t.Fatal(err)
	}
	title := "{title} | Help"
	if _, err := rig.svc.UpdatePageTypeStructure(ctx, UpdatePageTypeStructureInput{ID: pt.ID, DefaultMetaTitle: &title}); err != nil {
		t.Fatal(err)
	}
	got, err := rig.svc.GetPage(ctx, p.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got.URLPath != "help-"+s+"/"+p.Slug {
		t.Errorf("url_path = %q, want it resynced to the type's root", got.URLPath)
	}
}

func TestAChannelRootChangeMovesThatChannelsURLs(t *testing.T) {
	rig := newURLRig(t)
	ctx := context.Background()
	s := sfx()
	var ch uuid.UUID
	if err := rig.conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'cms', 'USD') RETURNING id`, "cms-"+s).Scan(&ch); err != nil {
		t.Fatal(err)
	}
	pt := rooted(t, rig.svc, "g-"+s)
	p := seedPage(t, rig.svc, pt, "pg-"+s, nil)
	if _, err := rig.svc.SetPageTypeChannelRoot(ctx, pt.ID, ch, "c-"+s); err != nil {
		t.Fatal(err)
	}
	var path string
	if err := rig.conn.QueryRow(`SELECT request_path FROM url_rewrites WHERE entity_id = $1 AND channel_id = $2 AND redirect_type = 0`, p.ID, ch).Scan(&path); err != nil {
		t.Fatalf("channel canonical: %v", err)
	}
	if path != "c-"+s+"/"+p.Slug {
		t.Errorf("channel canonical = %q", path)
	}
	if _, err := rig.svc.SetPageTypeChannelRoot(ctx, pt.ID, uuid.New(), "x-"+s); !errors.Is(err, ErrChannelNotFound) {
		t.Errorf("an unknown channel = %v, want ErrChannelNotFound", err)
	}
}

func TestUpdatePageRollsBackWhenADescendantPathClashes(t *testing.T) {
	rig := newURLRig(t)
	ctx := context.Background()
	s := sfx()
	pt := seedPageType(t, rig.svc)
	parent := seedPage(t, rig.svc, pt, "p-"+s, nil)
	child := seedPage(t, rig.svc, pt, "kid-"+s, &parent.ID)
	other := seedPage(t, rig.svc, pt, "taken-"+s, nil)
	if _, err := rig.conn.Exec(`UPDATE url_rewrites SET request_path = $2 WHERE entity_id = $1 AND redirect_type = 0`, other.ID, "moved-"+s+"/kid-"+s); err != nil {
		t.Fatal(err)
	}

	if _, err := rig.svc.UpdatePage(ctx, UpdatePageInput{ID: parent.ID, Slug: ptrTo("moved-" + s)}); !errors.Is(err, ErrURLPathConflict) {
		t.Fatalf("rename into a clashing child path = %v, want ErrURLPathConflict", err)
	}
	got, err := rig.svc.GetPage(ctx, parent.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != parent.Slug || rig.canonical(t, parent.ID) != parent.Slug || rig.canonical(t, child.ID) != parent.Slug+"/kid-"+s {
		t.Errorf("slug %q canonical %q - the refused rename half-applied", got.Slug, rig.canonical(t, parent.ID))
	}
}

func ptrTo[T any](v T) *T { return &v }

func TestAPageCannotMoveUnderItself(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	root := seedPage(t, svc, pt, "root-"+sfx(), nil)
	child := seedPage(t, svc, pt, "child-"+sfx(), &root.ID)
	grandchild := seedPage(t, svc, pt, "grandchild-"+sfx(), &child.ID)
	for name, parent := range map[string]uuid.UUID{"itself": root.ID, "its child": child.ID, "its grandchild": grandchild.ID} {
		if _, err := svc.UpdatePage(ctx, UpdatePageInput{ID: root.ID, ParentID: &parent}); !errors.Is(err, ErrPageCycle) {
			t.Errorf("parent = %s: err = %v, want ErrPageCycle", name, err)
		}
	}
	if _, err := svc.UpdatePage(ctx, UpdatePageInput{ID: grandchild.ID, ParentID: &root.ID}); err != nil {
		t.Errorf("a legal move was refused: %v", err)
	}
}

func TestDeletingAParentIsRefusedAndBulkDeleteCountsRealRows(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	parent := seedPage(t, svc, pt, "par-"+sfx(), nil)
	seedPage(t, svc, pt, "kid-"+sfx(), &parent.ID)
	if err := svc.DeletePage(ctx, parent.ID); !errors.Is(err, ErrPageHasChildren) {
		t.Errorf("deleting a parent = %v, want ErrPageHasChildren", err)
	}
	lone := seedPage(t, svc, pt, "lone-"+sfx(), nil)
	n, err := svc.BulkDelete(ctx, []uuid.UUID{lone.ID, uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("bulk delete counted %d, want the 1 row it removed", n)
	}
}

func TestAManualFlipConsumesThePendingSchedule(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	p := seedPage(t, svc, seedPageType(t, svc), "flip-"+sfx(), nil)
	past := time.Now().Add(-time.Minute)
	if _, err := svc.SchedulePage(ctx, p.ID, &past); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishPage(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UnpublishPage(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.PromoteDuePages(ctx); err != nil || n != 0 {
		t.Fatalf("scheduler tick promoted %d (%v), want nothing", n, err)
	}
	got, err := svc.GetPage(ctx, p.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsPublished || got.ScheduledAt != nil {
		t.Errorf("published=%v scheduled=%v - the unpublished page came back on the old schedule", got.IsPublished, got.ScheduledAt)
	}
	if _, err := svc.PublishPage(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if _, err := svc.SchedulePage(ctx, p.ID, &later); err != nil {
		t.Fatal(err)
	}
	if got, err = svc.UnpublishPage(ctx, p.ID); err != nil || got.ScheduledAt != nil {
		t.Errorf("unpublish left scheduled_at = %v (%v), so the scheduler would bring it back", got.ScheduledAt, err)
	}
	if _, err := svc.SchedulePage(ctx, uuid.New(), &past); !errors.Is(err, ErrPageNotFound) {
		t.Errorf("scheduling an unknown page = %v, want ErrPageNotFound", err)
	}
}

func TestAScheduleClearedDuringTheVetoIsNotPublished(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	p := seedPage(t, svc, seedPageType(t, svc), "veto-"+sfx(), nil)
	past := time.Now().Add(-time.Minute)
	if _, err := svc.SchedulePage(ctx, p.ID, &past); err != nil {
		t.Fatal(err)
	}
	svc.bus.SubscribeSync(EventPagePublishing, 1, func(ctx context.Context, e events.Event) error {
		_, err := svc.SchedulePage(ctx, e.(*PagePublishing).PageID, nil)
		return err
	})
	n, err := svc.PromoteDuePages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetPage(ctx, p.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || got.IsPublished {
		t.Errorf("promoted %d, published=%v - a schedule cleared mid-run still went live", n, got.IsPublished)
	}
}

func TestApprovalNeedsReviewAnotherAdminAndSurvivesNoEdit(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	p := seedPage(t, svc, seedPageType(t, svc), "wf-"+sfx(), nil)
	author := auth.WithAdmin(ctx, &auth.Claims{AdminID: uuid.New()})
	reviewer := auth.WithAdmin(ctx, &auth.Claims{AdminID: uuid.New()})

	if _, err := svc.ApprovePage(reviewer, p.ID); !errors.Is(err, ErrNotInReview) {
		t.Errorf("approving a draft = %v, want ErrNotInReview", err)
	}
	if _, err := svc.pageWorkflowService.repo.ApprovePageWorkflow(ctx, p.ID, nil); !errors.Is(err, ErrNotInReview) {
		t.Errorf("the approve write on a draft = %v, want ErrNotInReview - it must hold even when the service check races", err)
	}
	if _, err := svc.SubmitPageForReview(author, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApprovePage(author, p.ID); !errors.Is(err, ErrSelfApproval) {
		t.Errorf("self approval = %v, want ErrSelfApproval", err)
	}
	approved, err := svc.ApprovePage(reviewer, p.ID)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.WorkflowState != WorkflowApproved {
		t.Fatalf("state = %s, want approved", approved.WorkflowState)
	}
	edited, err := svc.UpsertPageContent(author, p.ID, PageContentInput{Locale: "en", Title: "Changed after approval"})
	if err != nil {
		t.Fatal(err)
	}
	if edited.WorkflowState != WorkflowDraft || edited.ApprovedBy != nil {
		t.Errorf("state = %s approvedBy = %v - an edit after approval kept the approval", edited.WorkflowState, edited.ApprovedBy)
	}
	if _, err := svc.SubmitPageForReview(author, uuid.New()); !errors.Is(err, ErrPageNotFound) {
		t.Errorf("submitting an unknown page = %v, want ErrPageNotFound", err)
	}
}

func TestBodiesClaimingToBeDocumentsMustParse(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	p := seedPage(t, svc, pt, "xss-"+sfx(), nil)
	bad := json.RawMessage(`{"version":"1","blocks":[{"id":"x","type":"html","props":{"html":"<img src=x onerror=alert(1)>"}}]}`)
	if _, err := svc.UpsertPageContent(ctx, p.ID, PageContentInput{Locale: "en", Title: "x", Body: bad}); !errors.Is(err, blockkit.ErrInvalidDocument) {
		t.Errorf("content = %v, want ErrInvalidDocument", err)
	}
	if _, err := svc.UpsertPageVariantBody(ctx, p.ID, "en", "b", bad); !errors.Is(err, blockkit.ErrInvalidDocument) {
		t.Errorf("variant = %v, want ErrInvalidDocument", err)
	}
	if _, err := svc.CreatePage(ctx, CreatePageInput{PageTypeID: pt.ID, Slug: "xss2-" + sfx(), Locale: "en", Title: "x", Body: bad}); !errors.Is(err, blockkit.ErrInvalidDocument) {
		t.Errorf("create = %v, want ErrInvalidDocument", err)
	}
	if _, err := svc.UpsertPageContent(ctx, p.ID, PageContentInput{Locale: "en", Title: "x", Body: json.RawMessage(`{"root":{"children":[]}}`)}); err != nil {
		t.Errorf("a freeform body was refused: %v", err)
	}
}

func TestSnippetsAndBlockTypesValidateAndPatchPartially(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	doc := json.RawMessage(`{"version":1,"blocks":[{"id":"a","type":"hero"}]}`)
	if _, err := svc.CreateSnippet(ctx, "sn-"+sfx(), "Promo", json.RawMessage(`{"version":1,"blocks":[{"type":"hero"}]}`)); !errors.Is(err, blockkit.ErrInvalidDocument) {
		t.Errorf("an invalid snippet = %v, want ErrInvalidDocument", err)
	}
	if _, err := svc.CreateSnippet(ctx, "sn-"+sfx(), "  ", doc); !errors.Is(err, ErrRequiredFieldMissing) {
		t.Errorf("a nameless snippet = %v, want ErrRequiredFieldMissing", err)
	}
	sn, err := svc.CreateSnippet(ctx, "sn-"+sfx(), "Promo", doc)
	if err != nil {
		t.Fatal(err)
	}
	if sn, err = svc.UpdateSnippet(ctx, sn.ID, nil, json.RawMessage(`{"version":1,"blocks":[]}`)); err != nil {
		t.Fatal(err)
	}
	if sn.Name != "Promo" {
		t.Errorf("a blocks-only update blanked the name to %q", sn.Name)
	}

	if _, err := svc.CreateBlockType(ctx, blockkit.TypeHTML, "Mine", "", nil, "<p>x</p>"); !errors.Is(err, ErrReservedBlockKey) {
		t.Errorf("a built-in key = %v, want ErrReservedBlockKey", err)
	}
	bt, err := svc.CreateBlockType(ctx, "card-"+sfx(), "Card", "box", nil, "<p>{{title}}</p>")
	if err != nil {
		t.Fatal(err)
	}
	renamed := "Card v2"
	if bt, err = svc.UpdateBlockType(ctx, bt.ID, BlockTypePatch{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	if bt.HTMLTemplate != "<p>{{title}}</p>" || bt.Icon != "box" || bt.Name != renamed {
		t.Errorf("a name-only update left %+v", bt)
	}
}

func TestLivePageEditsAnnounceAChange(t *testing.T) {
	rig := newURLRig(t)
	ctx := context.Background()
	pt := seedPageType(t, rig.svc)
	p := seedPage(t, rig.svc, pt, "live-"+sfx(), nil)
	if _, err := rig.svc.PublishPage(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	var (
		mu  sync.Mutex
		got []*PageChanged
	)
	rig.bus.Subscribe(EventPageChanged, func(_ context.Context, e events.Event) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, e.(*PageChanged))
		return nil
	})
	if _, err := rig.svc.UpsertPageContent(ctx, p.ID, PageContentInput{Locale: "en", Title: "New"}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, rig.bus)
	if err := rig.svc.DeletePage(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, rig.bus)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("got %d page.changed events, want one per edit", len(got))
	}
	if len(got[1].Paths) == 0 || got[1].Paths[0] != "/"+p.URLPath {
		t.Errorf("delete announced paths %v, want the page's old URL", got[1].Paths)
	}
}

func TestStructureChecksAreEnforced(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	pt := seedPageType(t, svc)
	seedPage(t, svc, pt, "one-"+sfx(), nil)
	seedPage(t, svc, pt, "two-"+sfx(), nil)
	yes := true
	if _, err := svc.UpdatePageTypeStructure(ctx, UpdatePageTypeStructureInput{ID: pt.ID, IsSingleton: &yes}); !errors.Is(err, ErrSingletonExists) {
		t.Errorf("singleton over two pages = %v, want ErrSingletonExists", err)
	}

	target := TargetCMSPage
	rel, err := svc.CreateAttribute(ctx, CreateAttributeInput{Slug: "rt-" + sfx(), InputType: InputRelationship, RelationshipTarget: &target})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AssignAttribute(ctx, AssignAttributeInput{PageTypeID: pt.ID, AttributeID: rel.ID}); err != nil {
		t.Fatal(err)
	}
	src := seedPage(t, svc, pt, "src-"+sfx(), nil)
	dst := seedPage(t, svc, pt, "dst-"+sfx(), nil)
	if _, err := svc.SetPageRelationships(ctx, src.ID, rel.ID, []PageRelationship{{TargetType: TargetCMSPage, TargetID: dst.ID}}); err != nil {
		t.Fatal(err)
	}
	product := TargetProduct
	if _, err := svc.UpdateAttribute(ctx, UpdateAttributeInput{ID: rel.ID, RelationshipTarget: &product}); !errors.Is(err, ErrAttributeInUse) {
		t.Errorf("retargeting a used relationship = %v, want ErrAttributeInUse", err)
	}
	bogus := RelationshipTarget("order")
	if _, err := svc.UpdateAttribute(ctx, UpdateAttributeInput{ID: rel.ID, RelationshipTarget: &bogus}); !errors.Is(err, ErrInvalidRelationship) {
		t.Errorf("an unknown target = %v, want ErrInvalidRelationship", err)
	}
}

func TestTagsShareOneNormalizer(t *testing.T) {
	svc, _, _ := newDBService(t, false)
	ctx := context.Background()
	p := seedPage(t, svc, seedPageType(t, svc), "tags-"+sfx(), nil)
	tag, err := svc.CreateTag(ctx, "Summer Sale!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BulkTag(ctx, []uuid.UUID{p.ID}, []string{"  Summer Sale! "}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetPage(ctx, p.ID, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != tag.Slug {
		t.Errorf("page tags %v, want the vocabulary slug %q", got.Tags, tag.Slug)
	}
	if err := svc.DeleteTag(ctx, tag.Slug); err != nil {
		t.Fatal(err)
	}
	if got, _ = svc.GetPage(ctx, p.ID, "en"); len(got.Tags) != 0 {
		t.Errorf("tags %v survived the tag's deletion", got.Tags)
	}
}
