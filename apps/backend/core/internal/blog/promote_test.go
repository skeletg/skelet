package blog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// Embeds postStore so an unplanned call panics
type fakePostStore struct {
	postStore
	posts map[uuid.UUID]Post
	due   []uuid.UUID
}

func (f *fakePostStore) ListDueScheduledPosts(context.Context) ([]Post, error) {
	var out []Post
	for _, id := range f.due {
		out = append(out, f.posts[id])
	}
	return out, nil
}

func (f *fakePostStore) SetPostStatus(_ context.Context, id uuid.UUID, status PostStatus, at *time.Time) (Post, error) {
	p := f.posts[id]
	p.Status, p.PublishedAt = status, at
	f.posts[id] = p
	return p, nil
}

type seenEvents struct {
	mu    sync.Mutex
	types []string
}

func (l *seenEvents) record(_ context.Context, e events.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.types = append(l.types, e.Type())
	return nil
}

func scheduledPost() Post {
	at := time.Now().Add(-time.Hour)
	return Post{ID: uuid.New(), Slug: "launch", Status: PostStatusScheduled, PublishedAt: &at}
}

func newPromoteHarness(posts ...Post) (*postService, *fakePostStore, *events.Bus, *seenEvents) {
	store := &fakePostStore{posts: map[uuid.UUID]Post{}}
	for _, p := range posts {
		store.posts[p.ID] = p
		store.due = append(store.due, p.ID)
	}
	bus := events.NewBus()
	log := &seenEvents{}
	bus.SubscribeSync(EventPostPublishing, 0, log.record)
	bus.Subscribe(EventPostPublished, log.record)
	return &postService{core: &core{bus: bus}, repo: store}, store, bus, log
}

func TestPromoteDuePostsAsksThePublishingHooks(t *testing.T) {
	ps, _, bus, log := newPromoteHarness(scheduledPost())
	if _, err := ps.PromoteDuePosts(context.Background()); err != nil {
		t.Fatalf("promote: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.WaitIdle(ctx); err != nil {
		t.Fatal(err)
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if len(log.types) != 2 || log.types[0] != EventPostPublishing || log.types[1] != EventPostPublished {
		t.Errorf("events = %v, want publishing then published - a scheduled post must pass the same veto a manual one does", log.types)
	}
}

func TestPromoteDuePostsHoldsAVetoedPostAndPromotesTheRest(t *testing.T) {
	vetoed, fine := scheduledPost(), scheduledPost()
	ps, store, bus, _ := newPromoteHarness(vetoed, fine)
	bus.SubscribeSync(EventPostPublishing, 1, func(_ context.Context, e events.Event) error {
		if e.(*PostPublishing).PostID == vetoed.ID {
			return errors.New("legal review pending")
		}
		return nil
	})
	n, err := ps.PromoteDuePosts(context.Background())
	if err != nil {
		t.Fatalf("a veto must not fail the whole run: %v", err)
	}
	if n != 1 {
		t.Errorf("promoted %d, want 1", n)
	}
	if got := store.posts[vetoed.ID]; got.Status != PostStatusScheduled || got.PublishedAt == nil || !got.PublishedAt.Equal(*vetoed.PublishedAt) {
		t.Errorf("vetoed post: status=%s published_at=%v, want it still scheduled for its original time", got.Status, got.PublishedAt)
	}
	if store.posts[fine.ID].Status != PostStatusPublished {
		t.Error("the post nobody vetoed was held back too")
	}
}
