package urlrewrite

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

type countingRepo struct {
	Repository
	reads map[string]int
}

func (r *countingRepo) GetByRequestPath(_ context.Context, path string, channelID *uuid.UUID) (UrlRewrite, error) {
	r.reads[cacheKey(channelID)]++
	return UrlRewrite{RequestPath: path, ChannelID: channelID}, nil
}

func TestAGlobalRewriteDropsEveryChannelsCachedAnswerAndAChannelOneOnlyItsOwn(t *testing.T) {
	dsn := os.Getenv("VALKEY_URL")
	if dsn == "" {
		t.Skip("set VALKEY_URL to exercise the cache")
	}
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	c := cache.New(client, "test-urlrewrite-"+uuid.NewString())
	repo := &countingRepo{reads: map[string]int{}}
	s := &Service{repo: repo, cache: c}
	a, b := uuid.New(), uuid.New()
	path := "tires/" + uuid.NewString()
	resolveAll := func() {
		for _, ch := range []*uuid.UUID{nil, &a, &b} {
			if _, err := s.ResolveURLForChannel(ctx, path, ch); err != nil {
				t.Fatal(err)
			}
		}
	}
	resolveAll()
	resolveAll()
	if repo.reads[cacheKey(&a)] != 1 {
		t.Fatalf("channel A read the database %d times, want once and then the cache", repo.reads[cacheKey(&a)])
	}
	s.invalidatePath(ctx, path, &a)
	resolveAll()
	if repo.reads[cacheKey(&a)] != 2 || repo.reads[cacheKey(&b)] != 1 {
		t.Fatalf("after a channel A write: A read %d, B read %d, want 2 and 1", repo.reads[cacheKey(&a)], repo.reads[cacheKey(&b)])
	}
	s.invalidatePath(ctx, path, nil)
	resolveAll()
	if repo.reads[cacheKey(nil)] != 2 || repo.reads[cacheKey(&a)] != 3 || repo.reads[cacheKey(&b)] != 2 {
		t.Fatalf("after a global write the reads were %v, want every scope read again", repo.reads)
	}
}
