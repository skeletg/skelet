package cms

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Embeds pageStore so a call the test didn't plan for panics on the nil interface instead of passing silently
type fakePageStore struct {
	pageStore
	pages       map[uuid.UUID]Page
	due         []uuid.UUID
	publishes   int
	unschedules int
}

func newFakePageStore(pages ...Page) *fakePageStore {
	f := &fakePageStore{pages: map[uuid.UUID]Page{}}
	for _, p := range pages {
		f.pages[p.ID] = p
	}
	return f
}

func (f *fakePageStore) GetPageByID(_ context.Context, id uuid.UUID) (Page, error) {
	p, ok := f.pages[id]
	if !ok {
		return Page{}, ErrPageNotFound
	}
	return p, nil
}

func (f *fakePageStore) SetPagePublished(_ context.Context, id uuid.UUID, published bool, at *time.Time) (Page, error) {
	p := f.pages[id]
	p.IsPublished, p.PublishedAt = published, at
	f.pages[id] = p
	f.publishes++
	return p, nil
}

func (f *fakePageStore) SetPageWorkflowState(_ context.Context, id uuid.UUID, state WorkflowState) (Page, error) {
	p := f.pages[id]
	p.WorkflowState = state
	f.pages[id] = p
	return p, nil
}

func (f *fakePageStore) ListDueScheduledPages(context.Context) ([]Page, error) {
	var out []Page
	for _, id := range f.due {
		out = append(out, f.pages[id])
	}
	return out, nil
}

func (f *fakePageStore) SetScheduledAt(_ context.Context, id uuid.UUID, at *time.Time) (Page, error) {
	p := f.pages[id]
	p.ScheduledAt = at
	f.pages[id] = p
	if at == nil {
		f.unschedules++
	}
	return p, nil
}

type fakeApprovalConfig bool

func (c fakeApprovalConfig) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	if path != cfgRequireApproval {
		return false, errors.New("unexpected config path " + path)
	}
	return bool(c), nil
}

type eventLog struct {
	mu    sync.Mutex
	types []string
}

func (l *eventLog) record(_ context.Context, e events.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.types = append(l.types, e.Type())
	return nil
}

func (l *eventLog) seen() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.types...)
}

func newPublishHarness(t *testing.T, requireApproval bool, pages ...Page) (*pageService, *fakePageStore, *events.Bus, *eventLog) {
	t.Helper()
	store := newFakePageStore(pages...)
	bus := events.NewBus()
	log := &eventLog{}
	bus.SubscribeSync(EventPagePublishing, 0, log.record)
	bus.Subscribe(EventPagePublished, log.record)
	bus.Subscribe(EventPageUnpublished, log.record)
	ps := &pageService{core: &core{bus: bus, cfg: fakeApprovalConfig(requireApproval)}, repo: store}
	return ps, store, bus, log
}

func waitIdle(t *testing.T, bus *events.Bus) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.WaitIdle(ctx); err != nil {
		t.Fatalf("bus never went idle: %v", err)
	}
}

func draftPage(state WorkflowState) Page {
	return Page{ID: uuid.New(), PageTypeID: uuid.New(), Slug: "about", URLPath: "about", WorkflowState: state}
}

func TestPublishPageApprovalGateRefusesUnapprovedStates(t *testing.T) {
	for _, state := range []WorkflowState{WorkflowDraft, WorkflowInReview} {
		t.Run(string(state), func(t *testing.T) {
			p := draftPage(state)
			ps, store, bus, log := newPublishHarness(t, true, p)
			if _, err := ps.PublishPage(context.Background(), p.ID); !errors.Is(err, ErrApprovalRequired) {
				t.Fatalf("err = %v, want ErrApprovalRequired", err)
			}
			waitIdle(t, bus)
			if store.publishes != 0 || store.pages[p.ID].IsPublished {
				t.Error("a refused publish still wrote is_published")
			}
			if got := log.seen(); len(got) != 0 {
				t.Errorf("a refused publish fired %v", got)
			}
		})
	}
}

func TestPublishPageApprovalGateAdmitsApprovedAndPublished(t *testing.T) {
	for _, state := range []WorkflowState{WorkflowApproved, WorkflowPublished} {
		t.Run(string(state), func(t *testing.T) {
			p := draftPage(state)
			ps, store, _, _ := newPublishHarness(t, true, p)
			got, err := ps.PublishPage(context.Background(), p.ID)
			if err != nil {
				t.Fatalf("publish: %v", err)
			}
			if !got.IsPublished || got.WorkflowState != WorkflowPublished {
				t.Errorf("got published=%v state=%s, want live and published", got.IsPublished, got.WorkflowState)
			}
			if store.pages[p.ID].PublishedAt == nil {
				t.Error("publish left published_at empty")
			}
		})
	}
}

func TestPublishPageWithoutGatePublishesDrafts(t *testing.T) {
	p := draftPage(WorkflowDraft)
	ps, _, bus, log := newPublishHarness(t, false, p)
	if _, err := ps.PublishPage(context.Background(), p.ID); err != nil {
		t.Fatalf("publish with the gate off: %v", err)
	}
	waitIdle(t, bus)
	want := []string{EventPagePublishing, EventPagePublished}
	if got := log.seen(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestPublishPageNilConfigMeansFreePublish(t *testing.T) {
	p := draftPage(WorkflowDraft)
	ps := &pageService{core: &core{}, repo: newFakePageStore(p)}
	if _, err := ps.PublishPage(context.Background(), p.ID); err != nil {
		t.Fatalf("publish with no config reader wired: %v", err)
	}
}

func TestPublishPageVetoWritesNothing(t *testing.T) {
	p := draftPage(WorkflowApproved)
	ps, store, bus, _ := newPublishHarness(t, true, p)
	veto := errors.New("content lockdown")
	bus.SubscribeSync(EventPagePublishing, 1, func(context.Context, events.Event) error { return veto })
	if _, err := ps.PublishPage(context.Background(), p.ID); !errors.Is(err, veto) {
		t.Fatalf("err = %v, want the veto", err)
	}
	if store.publishes != 0 || store.pages[p.ID].WorkflowState != WorkflowApproved {
		t.Error("a vetoed publish still changed the page")
	}
}

func TestPublishPageKeepsFirstPublishedAtAndStaysQuietWhenLive(t *testing.T) {
	first := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	p := draftPage(WorkflowPublished)
	p.IsPublished, p.PublishedAt = true, &first
	ps, store, bus, log := newPublishHarness(t, true, p)
	if _, err := ps.PublishPage(context.Background(), p.ID); err != nil {
		t.Fatalf("republish: %v", err)
	}
	waitIdle(t, bus)
	if at := store.pages[p.ID].PublishedAt; at == nil || !at.Equal(first) {
		t.Errorf("published_at = %v, want the original %v", at, first)
	}
	if got := log.seen(); len(got) != 0 {
		t.Errorf("republishing a live page fired %v", got)
	}
}

func TestUnpublishPageFiresOnlyWhenLive(t *testing.T) {
	live := draftPage(WorkflowPublished)
	live.IsPublished = true
	idle := draftPage(WorkflowDraft)
	ps, store, bus, log := newPublishHarness(t, false, live, idle)
	for _, id := range []uuid.UUID{live.ID, idle.ID} {
		if _, err := ps.UnpublishPage(context.Background(), id); err != nil {
			t.Fatalf("unpublish: %v", err)
		}
	}
	waitIdle(t, bus)
	if store.pages[live.ID].IsPublished {
		t.Error("the live page is still published")
	}
	if got := log.seen(); len(got) != 1 || got[0] != EventPageUnpublished {
		t.Errorf("events = %v, want one %s", got, EventPageUnpublished)
	}
}

func TestPromoteDuePagesHoldsUnapprovedPagesUnderTheGate(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	approved := draftPage(WorkflowApproved)
	approved.ScheduledAt = &past
	draft := draftPage(WorkflowDraft)
	draft.ScheduledAt = &past
	ps, store, bus, log := newPublishHarness(t, true, approved, draft)
	store.due = []uuid.UUID{approved.ID, draft.ID}

	n, err := ps.PromoteDuePages(context.Background())
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	waitIdle(t, bus)
	if n != 1 {
		t.Errorf("promoted %d, want 1", n)
	}
	if got := store.pages[approved.ID]; !got.IsPublished || got.ScheduledAt != nil || got.WorkflowState != WorkflowPublished {
		t.Errorf("approved page: published=%v scheduled=%v state=%s", got.IsPublished, got.ScheduledAt, got.WorkflowState)
	}
	if got := store.pages[draft.ID]; got.IsPublished || got.ScheduledAt == nil {
		t.Error("the unapproved page went live or lost its schedule - it must wait for approval")
	}
	want := []string{EventPagePublishing, EventPagePublished}
	if got := log.seen(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("events = %v, want %v for the approved page only", got, want)
	}
}

func TestPromoteDuePagesWithoutGatePromotesEverything(t *testing.T) {
	a, b := draftPage(WorkflowDraft), draftPage(WorkflowInReview)
	ps, store, _, _ := newPublishHarness(t, false, a, b)
	store.due = []uuid.UUID{a.ID, b.ID}
	n, err := ps.PromoteDuePages(context.Background())
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if n != 2 || store.unschedules != 2 {
		t.Errorf("promoted %d with %d schedules cleared, want 2 and 2", n, store.unschedules)
	}
}

func TestPromoteDuePagesAsksThePublishingHooks(t *testing.T) {
	p := draftPage(WorkflowApproved)
	ps, store, bus, log := newPublishHarness(t, true, p)
	store.due = []uuid.UUID{p.ID}
	if _, err := ps.PromoteDuePages(context.Background()); err != nil {
		t.Fatalf("promote: %v", err)
	}
	waitIdle(t, bus)
	want := []string{EventPagePublishing, EventPagePublished}
	if got := log.seen(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("events = %v, want %v - a scheduled publish must pass the same veto a manual one does", got, want)
	}
}

func TestPromoteDuePagesHoldsAVetoedPageAndPromotesTheRest(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	vetoed, fine := draftPage(WorkflowApproved), draftPage(WorkflowApproved)
	vetoed.ScheduledAt, fine.ScheduledAt = &past, &past
	ps, store, bus, _ := newPublishHarness(t, true, vetoed, fine)
	store.due = []uuid.UUID{vetoed.ID, fine.ID}
	bus.SubscribeSync(EventPagePublishing, 1, func(_ context.Context, e events.Event) error {
		if e.(*PagePublishing).PageID == vetoed.ID {
			return errors.New("content lockdown")
		}
		return nil
	})

	n, err := ps.PromoteDuePages(context.Background())
	if err != nil {
		t.Fatalf("a veto must not fail the whole run: %v", err)
	}
	if n != 1 {
		t.Errorf("promoted %d, want 1", n)
	}
	if got := store.pages[vetoed.ID]; got.IsPublished || got.ScheduledAt == nil || got.WorkflowState != WorkflowApproved {
		t.Errorf("vetoed page: published=%v scheduled=%v state=%s, want it untouched and still scheduled", got.IsPublished, got.ScheduledAt, got.WorkflowState)
	}
	if !store.pages[fine.ID].IsPublished {
		t.Error("the page nobody vetoed was held back too")
	}
}

func TestPromoteDuePagesKeepsFirstPublishedAt(t *testing.T) {
	first := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	past := time.Now().Add(-time.Hour)
	returning, fresh := draftPage(WorkflowApproved), draftPage(WorkflowApproved)
	returning.PublishedAt, returning.ScheduledAt = &first, &past
	fresh.ScheduledAt = &past
	ps, store, _, _ := newPublishHarness(t, true, returning, fresh)
	store.due = []uuid.UUID{returning.ID, fresh.ID}

	if _, err := ps.PromoteDuePages(context.Background()); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if at := store.pages[returning.ID].PublishedAt; at == nil || !at.Equal(first) {
		t.Errorf("rescheduled page published_at = %v, want the original %v", at, first)
	}
	if at := store.pages[fresh.ID].PublishedAt; at == nil || !at.Equal(past) {
		t.Errorf("first scheduled publish published_at = %v, want the scheduled time %v", at, past)
	}
}
