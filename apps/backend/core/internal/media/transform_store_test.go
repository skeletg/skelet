package media

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
)

type fakeStore struct {
	mu        sync.Mutex
	objects   map[string][]byte
	types     map[string]string
	gate      chan struct{} // when set, reading an original waits for it
	originals atomic.Int32
	heads     atomic.Int32
	reads     atomic.Int32
}

func newFakeStore() *fakeStore {
	return &fakeStore{objects: map[string][]byte{}, types: map[string]string{}}
}

func (f *fakeStore) GetBytes(ctx context.Context, key string) ([]byte, error) {
	f.originals.Add(1)
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", objstore.ErrObjectNotFound, key)
	}
	return b, nil
}

func (f *fakeStore) GetBytesTyped(_ context.Context, key string) ([]byte, string, error) {
	f.reads.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return nil, "", objstore.ErrObjectNotFound
	}
	return b, f.types[key], nil
}

func (f *fakeStore) PutBytes(_ context.Context, key string, data []byte, ctype string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key], f.types[key] = data, ctype
	return nil
}

func (f *fakeStore) Head(_ context.Context, key string) (int64, string, bool, error) {
	f.heads.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	return int64(len(b)), f.types[key], ok, nil
}

func (f *fakeStore) HasPublicBase() bool         { return false }
func (f *fakeStore) PublicURL(key string) string { return key }

// The shopper whose request started a render closing the tab failed every request queued on the same derivative
func TestAWaiterOutlivesTheRequestThatStartedTheRender(t *testing.T) {
	store := newFakeStore()
	store.objects["p/a.png"] = pngBytes(t, 800, 600)
	store.gate = make(chan struct{})
	tr := NewTransformer(store)

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leader := make(chan error, 1)
	go func() {
		_, _, err := tr.Transform(leaderCtx, "p/a.png", 320, 0, 0, 0, false)
		leader <- err
	}()
	waitFor(t, func() bool { return store.originals.Load() == 1 })

	waiter := make(chan error, 1)
	go func() {
		data, _, err := tr.Transform(context.Background(), "p/a.png", 320, 0, 0, 0, false)
		if err == nil && len(data) == 0 {
			err = errors.New("empty derivative")
		}
		waiter <- err
	}()
	waitFor(t, func() bool { return store.reads.Load() >= 3 })
	time.Sleep(20 * time.Millisecond) // the waiter is past its cache read and onto the shared render

	cancelLeader()
	if err := <-leader; !errors.Is(err, context.Canceled) {
		t.Fatalf("the cancelled caller = %v, want its own cancellation", err)
	}
	close(store.gate)
	if err := <-waiter; err != nil {
		t.Fatalf("the waiter = %v, want the derivative", err)
	}
	if n := store.originals.Load(); n != 1 {
		t.Errorf("the original was read %d times, want one shared render", n)
	}
}

func TestACacheHitIsOneRoundTrip(t *testing.T) {
	store := newFakeStore()
	store.objects[DerivedKey("p/a.png", 320, 0, false)] = []byte("cached")
	store.types[DerivedKey("p/a.png", 320, 0, false)] = "image/png"
	data, ctype, err := NewTransformer(store).Transform(context.Background(), "p/a.png", 320, 0, 0, 0, false)
	if err != nil || string(data) != "cached" || ctype != "image/png" {
		t.Fatalf("hit = %q %q %v", data, ctype, err)
	}
	if store.heads.Load() != 0 || store.reads.Load() != 1 {
		t.Errorf("a hit cost %d HEADs and %d reads, want one read", store.heads.Load(), store.reads.Load())
	}
}

// An iPhone HEIC upload can't be decoded here - answering it as a storage fault 502'd it uncacheably on every request
func TestAnUndecodableOriginalIsUnsupported(t *testing.T) {
	store := newFakeStore()
	store.objects["p/photo.heic"] = []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic")
	_, _, err := NewTransformer(store).Transform(context.Background(), "p/photo.heic", 320, 0, 0, 0, false)
	if !errors.Is(err, ErrUnsupportedSource) {
		t.Fatalf("HEIC = %v, want ErrUnsupportedSource", err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for range 500 {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition never held")
}
