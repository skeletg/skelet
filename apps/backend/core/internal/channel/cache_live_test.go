package channel_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

type countingChannelRepo struct {
	*svcRepo
	byID, byHost int
}

func (r *countingChannelRepo) GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error) {
	r.byID++
	return r.svcRepo.GetByID(ctx, id)
}

func (r *countingChannelRepo) GetByStorefrontHost(ctx context.Context, host string) (channel.Channel, error) {
	r.byHost++
	return r.svcRepo.GetByStorefrontHost(ctx, host)
}

func liveCache(t *testing.T) *cache.Cache {
	t.Helper()
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
	return cache.New(client, "test-channel-"+uuid.NewString())
}

func TestTheStorefrontsChannelLookupIsCachedAndAWriteRefreshesIt(t *testing.T) {
	ctx := context.Background()
	ch := channel.Channel{ID: uuid.New(), Slug: "a", Name: "Old", IsActive: true}
	repo := &countingChannelRepo{svcRepo: newSvcRepo(ch)}
	svc := channel.NewService(repo)
	svc.SetCache(liveCache(t))

	for range 3 {
		if _, err := svc.GetByID(ctx, ch.ID); err != nil {
			t.Fatal(err)
		}
	}
	if repo.byID != 1 {
		t.Fatalf("three lookups read the database %d times, want once", repo.byID)
	}
	name := "New"
	if _, err := svc.Update(ctx, channel.UpdateChannelInput{ID: ch.ID, Name: &name}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetByID(ctx, ch.ID)
	if err != nil || got.Name != "New" {
		t.Fatalf("after the write the lookup read %q, %v, want the new name", got.Name, err)
	}

	for range 3 {
		if _, err := svc.GetByStorefrontHost(ctx, "nobody.example"); !errors.Is(err, channel.ErrChannelNotFound) {
			t.Fatalf("unknown host = %v, want not found", err)
		}
	}
	if repo.byHost != 1 {
		t.Fatalf("three misses on one host scanned %d times, want once", repo.byHost)
	}
}
