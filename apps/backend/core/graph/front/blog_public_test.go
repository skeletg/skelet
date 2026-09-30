package front

import (
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
)

func cat(id string, published bool, children ...*model.BlogCategory) *model.BlogCategory {
	return &model.BlogCategory{ID: id, IsPublished: published, Children: children}
}

func TestAnUnpublishedCategoryReadsLikeAMissingOne(t *testing.T) {
	if _, err := publicCategory(cat("draft", false)); !errors.Is(err, blog.ErrCategoryNotFound) {
		t.Errorf("err = %v, want ErrCategoryNotFound", err)
	}
	if _, err := publicTag(&model.BlogTag{ID: "draft"}); !errors.Is(err, blog.ErrTagNotFound) {
		t.Errorf("err = %v, want ErrTagNotFound", err)
	}
}

// A published category can still carry a draft parent or draft children, and a published post draft categories and tags
func TestUnpublishedTaxonomyIsPrunedWhereverItNests(t *testing.T) {
	c := cat("live", true, cat("live-child", true, cat("draft-grandchild", false)), cat("draft-child", false))
	c.Parent = cat("draft-parent", false)
	got, err := publicCategory(c)
	if err != nil {
		t.Fatal(err)
	}
	if got.Parent != nil {
		t.Error("a draft parent was shown")
	}
	if len(got.Children) != 1 || got.Children[0].ID != "live-child" || len(got.Children[0].Children) != 0 {
		t.Errorf("children = %+v, want only live-child with no draft grandchild", got.Children)
	}
	if len(c.Children) != 2 {
		t.Error("pruning changed the shared model it was given")
	}

	post := publicPost(&model.BlogPost{
		Categories: []*model.BlogCategory{cat("a", true), cat("b", false)},
		Tags:       []*model.BlogTag{{ID: "t1", IsPublished: true}, {ID: "t2"}},
	})
	if len(post.Categories) != 1 || post.Categories[0].ID != "a" || len(post.Tags) != 1 || post.Tags[0].ID != "t1" {
		t.Errorf("post kept draft taxonomy: categories %+v, tags %+v", post.Categories, post.Tags)
	}
}

// The public author type once carried the author's email, and anyone could list it
func TestThePublicAuthorTypeHasNoEmail(t *testing.T) {
	sdl, err := os.ReadFile("../schema/front/blog.graphqls")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)type BlogAuthor \{(.*?)\}`).FindSubmatch(sdl)
	if block == nil {
		t.Fatal("no BlogAuthor type in the storefront schema")
	}
	if regexp.MustCompile(`(?m)^\s*email\s*:`).Match(block[1]) {
		t.Error("the storefront BlogAuthor exposes email again")
	}
}

func TestPublicEmailOnlyForAuthorsWhoOptedIn(t *testing.T) {
	addr, blank := "jane@example.com", ""
	r := &blogAuthorResolver{}
	for name, c := range map[string]struct {
		author *model.BlogAuthor
		want   *string
	}{
		"opted in":             {&model.BlogAuthor{Email: &addr, EmailPublic: true}, &addr},
		"not opted in":         {&model.BlogAuthor{Email: &addr}, nil},
		"opted in, no address": {&model.BlogAuthor{Email: &blank, EmailPublic: true}, nil},
		"opted in, nil email":  {&model.BlogAuthor{EmailPublic: true}, nil},
	} {
		got, err := r.PublicEmail(t.Context(), c.author)
		if err != nil || (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: PublicEmail = (%v, %v), want %v", name, got, err, c.want)
		}
	}
}
