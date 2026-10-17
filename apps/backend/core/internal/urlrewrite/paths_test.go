package urlrewrite

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestARedirectTargetStaysOnTheSite(t *testing.T) {
	for _, ok := range []string{"/sale", "/blog/post?x=1", "/"} {
		if !ValidTargetPath(ok) {
			t.Errorf("ValidTargetPath(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"//evil.com", "/\\evil.com", "https://evil.com", "evil.com", "/a\nb", "/a b"} {
		if ValidTargetPath(bad) {
			t.Errorf("ValidTargetPath(%q) = true, want false", bad)
		}
	}
	svc := &Service{repo: &fakeScopeRepo{}}
	evil := "//evil.com"
	if _, err := svc.Create(context.Background(), CreateInput{RequestPath: "old", TargetPath: &evil, RedirectType: RedirectTypePermanent}); !errors.Is(err, ErrInvalidTarget) {
		t.Errorf("Create(//evil.com) = %v, want ErrInvalidTarget", err)
	}
}

// The resolver looks up the bare form, so a redirect saved as /sale/ used to sit in the list and never match
func TestRequestPathsAreStoredBare(t *testing.T) {
	repo := &fakeScopeRepo{}
	svc := &Service{repo: repo}
	if _, err := svc.Create(context.Background(), CreateInput{RequestPath: " /sale/summer/ ", RedirectType: RedirectTypePermanent}); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateManualRedirect(context.Background(), "/old/", "product", uuid.New(), 301); err != nil {
		t.Fatal(err)
	}
	if repo.created[0].RequestPath != "sale/summer" || repo.created[1].RequestPath != "old" {
		t.Errorf("stored %q and %q, want sale/summer and old", repo.created[0].RequestPath, repo.created[1].RequestPath)
	}
	for _, bad := range []string{"", "/", "a?b=1", "a#top", "a b", "a//b", "a\\b"} {
		if _, err := NormalizeRequestPath(bad); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("NormalizeRequestPath(%q) = %v, want ErrInvalidPath", bad, err)
		}
	}
}

func TestUpdateCantFlipACanonical(t *testing.T) {
	redirect, canonical := uuid.New(), uuid.New()
	repo := &fakeScopeRepo{byID: map[uuid.UUID]UrlRewrite{
		redirect:  {ID: redirect, RequestPath: "old", RedirectType: RedirectTypePermanent},
		canonical: {ID: canonical, RequestPath: "p", RedirectType: RedirectTypeCanonical},
	}}
	svc := &Service{repo: repo}
	toCanonical, toRedirect := RedirectTypeCanonical, RedirectTypeTemporary
	if _, err := svc.Update(context.Background(), UpdateInput{ID: redirect, RedirectType: &toCanonical}); !errors.Is(err, ErrCanonicalChange) {
		t.Errorf("redirect -> canonical = %v, want ErrCanonicalChange", err)
	}
	if _, err := svc.Update(context.Background(), UpdateInput{ID: canonical, RedirectType: &toRedirect}); !errors.Is(err, ErrCanonicalChange) {
		t.Errorf("canonical -> redirect = %v, want ErrCanonicalChange", err)
	}
	if _, err := svc.Update(context.Background(), UpdateInput{ID: redirect, RedirectType: &toRedirect}); err != nil {
		t.Errorf("301 -> 302 = %v, want allowed", err)
	}
	evil := "/\\evil.com"
	if _, err := svc.Update(context.Background(), UpdateInput{ID: redirect, TargetPath: &evil}); !errors.Is(err, ErrInvalidTarget) {
		t.Errorf("Update(target /\\evil.com) = %v, want ErrInvalidTarget", err)
	}
}
