package embedding

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

func TestAFailedProviderRestsBeforeTheNextQuestion(t *testing.T) {
	ctx := context.Background()
	emb := &fakeEmbedder{model: "m1", err: errors.New("provider down")}
	s := NewSearcher(nil, nil, emb)
	if _, err := s.vector(ctx, uuid.Nil, "m1", "warm socks"); err == nil {
		t.Fatal("a failing provider answered")
	}
	emb.err = nil
	if _, err := s.vector(ctx, uuid.Nil, "m1", "rain jacket"); !errors.Is(err, errProviderResting) || len(emb.texts) != 0 {
		t.Errorf("right after a failure = %v with %d texts sent - want the provider rested", err, len(emb.texts))
	}
	s.resting.Store(0)
	if v, err := s.vector(ctx, uuid.Nil, "m1", "rain jacket"); err != nil || v == "" || len(emb.texts) != 1 {
		t.Errorf("once rested = %q, %v, %d texts sent", v, err, len(emb.texts))
	}
	if _, err := s.vector(ctx, uuid.Nil, "m1", "rain jacket"); err != nil || len(emb.texts) != 1 {
		t.Errorf("the same question again went to the provider: %d texts", len(emb.texts))
	}
}

type paceAfter struct {
	left  int
	keys  []string
	rates []int
}

// counts each shopper's questions only - the store's own pace always has room here
func (p *paceAfter) AllowN(_ context.Context, key string, rate int) bool {
	p.keys, p.rates = append(p.keys, key), append(p.rates, rate)
	if strings.HasPrefix(key, "meaning-store:") {
		return true
	}
	p.left--
	return p.left >= 0
}

func shopperAt(addr string) context.Context {
	req := httptest.NewRequest(http.MethodPost, "/api/front/graphql", nil)
	req.RemoteAddr = addr
	return auth.WithRequest(context.Background(), req)
}

func TestEachShopperAsksTheProviderAtTheirOwnPace(t *testing.T) {
	emb := &fakeEmbedder{model: "m1"}
	s := NewSearcher(nil, nil, emb)
	pace := &paceAfter{left: 1}
	store := uuid.New()
	s.SetPace(pace, func(_ context.Context, ch uuid.UUID) int {
		if ch != store {
			t.Errorf("read the pace of store %s", ch)
		}
		return 25
	}, func(context.Context, uuid.UUID) int { return 1200 })
	shopper := shopperAt("[2001:db8:1:2::7]:443")

	if _, err := s.vector(shopper, store, "m1", "warm socks"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.vector(shopperAt("[2001:db8:1:2::9]:443"), store, "m1", "rain jacket"); !errors.Is(err, ratelimit.ErrLimited) || len(emb.texts) != 1 {
		t.Errorf("a second new question from the same /64 = %v after %d texts sent - want it refused before the provider", err, len(emb.texts))
	}
	if pace.keys[0] != "meaning:"+store.String()+":2001:db8:1:2::/64" || pace.rates[0] != 25 {
		t.Errorf("counted under %q at %d a minute - want the store's own key and rate", pace.keys[0], pace.rates[0])
	}
	if _, err := s.vector(shopper, store, "m1", "warm socks"); err != nil {
		t.Errorf("a question already asked = %v - want it answered free from the cache", err)
	}
	if _, err := s.vector(ratelimit.Counted(shopper), store, "m1", "linen trousers"); err != nil {
		t.Errorf("a call its own limit counted = %v - want it left be", err)
	}
	if _, err := s.vector(context.Background(), store, "m1", "boots"); err != nil {
		t.Errorf("work no shopper asked for = %v - want no pace", err)
	}
}

type storeFull struct {
	keys  []string
	rates []int
}

func (p *storeFull) AllowN(_ context.Context, key string, rate int) bool {
	p.keys, p.rates = append(p.keys, key), append(p.rates, rate)
	return !strings.HasPrefix(key, "meaning-store:")
}

func TestAStoresNewQuestionsArePacedAndOnesAlreadyAskedAreFree(t *testing.T) {
	emb := &fakeEmbedder{model: "m1"}
	s := NewSearcher(nil, nil, emb)
	store := uuid.New()
	if _, err := s.vector(context.Background(), store, "m1", "warm socks"); err != nil {
		t.Fatal(err)
	}
	full := &storeFull{}
	s.SetPace(full, func(context.Context, uuid.UUID) int { return 100 }, func(context.Context, uuid.UUID) int { return 300 })
	if _, err := s.vector(shopperAt("192.0.2.1:443"), store, "m1", "warm socks"); err != nil || len(full.keys) != 0 {
		t.Errorf("a question already asked = %v, counted %v - want it free while the store is at its pace", err, full.keys)
	}
	if _, err := s.vector(shopperAt("192.0.2.1:443"), store, "m1", "rain jacket"); !errors.Is(err, ratelimit.ErrLimited) || len(emb.texts) != 1 {
		t.Errorf("a new question past the store's pace = %v after %d texts - want it answered on its words", err, len(emb.texts))
	}
	if last := len(full.keys) - 1; full.keys[last] != "meaning-store:"+store.String() || full.rates[last] != 300 {
		t.Errorf("counted under %v at %v - want the store's own key at the rate it set", full.keys, full.rates)
	}
}

// answers once let go, however long whoever asked first was willing to wait
type heldEmbedder struct {
	release chan struct{}
	mu      sync.Mutex
	asked   int
}

func (h *heldEmbedder) Model(context.Context) string { return "m1" }

func (h *heldEmbedder) Embed(ctx context.Context, _ ai.Purpose, texts []string) ([][]float32, int64, error) {
	h.mu.Lock()
	h.asked++
	h.mu.Unlock()
	select {
	case <-h.release:
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
	return [][]float32{make([]float32, ai.EmbeddingDimensions)}, 1, nil
}

func (h *heldEmbedder) wasAsked() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.asked > 0
}

func TestEachAskerWaitsOnlyAsLongAsItsOwnDeadline(t *testing.T) {
	emb := &heldEmbedder{release: make(chan struct{})}
	s := NewSearcher(nil, nil, emb)
	slow, cancel := context.WithTimeout(context.Background(), time.Minute) // the assistant's long budget asks first
	defer cancel()
	first := make(chan error, 1)
	go func() {
		_, err := s.vector(slow, uuid.Nil, "m1", "warm socks")
		first <- err
	}()
	for !emb.wasAsked() {
		time.Sleep(time.Millisecond)
	}
	quick, cancelQuick := context.WithTimeout(context.Background(), 50*time.Millisecond) // a storefront search joins it
	defer cancelQuick()
	began := time.Now()
	if _, err := s.vector(quick, uuid.Nil, "m1", "warm socks"); !errors.Is(err, context.DeadlineExceeded) || time.Since(began) > time.Second {
		t.Errorf("the storefront search = %v after %v - want it back at its own deadline", err, time.Since(began))
	}
	if s.resting.Load() != 0 {
		t.Error("a search that gave up rested the provider for everyone")
	}
	close(emb.release)
	if err := <-first; err != nil {
		t.Errorf("the first asker = %v - want the answer", err)
	}
}

func TestAnAskerGivingUpLeavesTheAnswerForTheNext(t *testing.T) {
	emb := &heldEmbedder{release: make(chan struct{})}
	s := NewSearcher(nil, nil, emb)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.vector(ctx, uuid.Nil, "m1", "warm socks"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("past its deadline = %v", err)
	}
	close(emb.release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := s.cache.get(questionKey("m1", "warm socks")); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the question was dropped with the asker who gave up")
		}
		time.Sleep(time.Millisecond)
	}
	if s.resting.Load() != 0 {
		t.Error("the provider was rested for a search that ran out of time")
	}
}
