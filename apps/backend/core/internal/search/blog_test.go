package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
)

type fakePosts struct{ posts map[uuid.UUID]blog.Post }

func (f *fakePosts) GetPost(_ context.Context, id uuid.UUID, _ string) (blog.Post, error) {
	return f.posts[id], nil
}
func (f *fakePosts) ListPosts(context.Context, blog.PostListFilter, string) ([]blog.Post, error) {
	out := make([]blog.Post, 0, len(f.posts))
	for _, p := range f.posts {
		out = append(out, p)
	}
	return out, nil
}

func blogFixture(t *testing.T, channels, posts int) (*BlogPostIndexer, *recordingCluster, *fakeChannels, *fakePosts) {
	t.Helper()
	cluster := &recordingCluster{}
	srv := httptest.NewServer(http.HandlerFunc(cluster.serve))
	t.Cleanup(srv.Close)
	client, err := NewClient([]string{srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	chs := &fakeChannels{}
	for range channels {
		chs.all = append(chs.all, channel.Channel{ID: uuid.New(), DefaultLocale: "en", Locales: []locale.Locale{{Code: "en", IsActive: true}, {Code: "de", IsActive: true}}})
	}
	src := &fakePosts{posts: map[uuid.UUID]blog.Post{}}
	for range posts {
		p := blog.Post{ID: uuid.New(), Slug: "post", Status: blog.PostStatusPublished,
			AllContent: []blog.PostContent{{Locale: "en", Title: "Hello", Body: json.RawMessage(`{"root":{"children":[{"type":"paragraph","children":[{"type":"text","text":"Body words"}]}]}}`)}}}
		for _, c := range chs.all {
			p.ChannelListings = append(p.ChannelListings, blog.PostChannelListing{ChannelID: c.ID, IsPublished: true})
		}
		src.posts[p.ID] = p
	}
	return NewBlogPostIndexer(client, src, chs, nil), cluster, chs, src
}

func TestABlogPostIndexesInOneBulkAndDropsItsStaleDocs(t *testing.T) {
	idx, cluster, chs, src := blogFixture(t, 3, 1)
	var pid uuid.UUID
	for id := range src.posts {
		pid = id
	}
	cluster.existing = []string{docID(pid, uuid.New(), "fr")}
	if err := idx.Upsert(context.Background(), pid); err != nil {
		t.Fatal(err)
	}
	for _, r := range cluster.requests {
		if strings.Contains(r, "/_doc/") {
			t.Fatalf("a doc went out on its own request %q", r)
		}
	}
	if cluster.count("index") != 6 || cluster.count("delete") != 1 {
		t.Fatalf("%d docs and %d deletes, want 6 docs and the stale one's delete", cluster.count("index"), cluster.count("delete"))
	}
	if chs.gets != 0 || chs.listings != 1 {
		t.Fatalf("read channels %d times one by one and listed %d times, want a single list", chs.gets, chs.listings)
	}
}

func TestABlogReindexListsChannelsOnce(t *testing.T) {
	idx, cluster, chs, _ := blogFixture(t, 2, 5)
	n, err := idx.ReindexAll(context.Background())
	if err != nil || n != 5 {
		t.Fatalf("reindex = %d, %v, want 5", n, err)
	}
	if chs.listings != 1 || chs.gets != 0 || cluster.count("index") != 20 {
		t.Fatalf("listings=%d gets=%d docs=%d, want 1/0/20", chs.listings, chs.gets, cluster.count("index"))
	}
}
