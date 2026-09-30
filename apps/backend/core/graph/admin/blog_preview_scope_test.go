package admin

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog/preview"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Answering "not found" before checking access told a channel-scoped admin which post ids exist on other channels
func TestPreviewMintRefusesOtherChannelsPostsLikeMissingOnes(t *testing.T) {
	conn := testdb.Require(t)
	mine, theirs := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{mine, theirs} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Preview','USD',true)`, ch, "pv-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	postID := uuid.New()
	if _, err := conn.Exec(`INSERT INTO blog_posts (id, slug) VALUES ($1, $2)`, postID, "pv-post-"+postID.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO blog_post_channel_listings (post_id, channel_id) VALUES ($1, $2)`, postID, theirs); err != nil {
		t.Fatal(err)
	}
	queries, tx := db.New(conn), txrunner.New(conn)
	r := &mutationResolver{&Resolver{
		BlogSvc:          blog.NewService(blog.Deps{Repo: blog.NewRepository(queries, tx), Tx: tx}),
		BlogPreviewStore: preview.NewStore(nil, 0),
	}}
	scoped := adminWithCachedGrants(auth.Grant{Slug: "blog.manage", ChannelID: &mine})

	_, existing := r.MintBlogPostPreviewToken(scoped, model.MintBlogPostPreviewTokenInput{PostID: postID.String()})
	_, missing := r.MintBlogPostPreviewToken(scoped, model.MintBlogPostPreviewTokenInput{PostID: uuid.NewString()})
	if existing == nil || missing == nil {
		t.Fatalf("errors = (%v, %v), want both refused", existing, missing)
	}
	if apperr.CodeOf(existing) != apperr.CodeOf(missing) || existing.Error() != missing.Error() {
		t.Errorf("another channel's post answered %q, a made-up id %q - they must read the same", existing, missing)
	}
}
