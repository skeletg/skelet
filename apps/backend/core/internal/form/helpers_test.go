package form

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

// The slug is the public handle the storefront resolves a form by, so it has to come out URL-safe
// whatever the admin typed as a name
func TestSlugify(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"already a slug":     {"newsletter", "newsletter"},
		"spaces":             {"Newsletter Signup", "newsletter-signup"},
		"mixed case":         {"NewsLetter", "newsletter"},
		"padded":             {"  Newsletter  ", "newsletter"},
		"punctuation":        {"Newsletter: 10% Off!", "newsletter-10-off"},
		"runs collapse":      {"a   ---   b", "a-b"},
		"leading separators": {"---newsletter", "newsletter"},
		"trailing":           {"newsletter---", "newsletter"},
		"digits kept":        {"Form 2026", "form-2026"},
		"accents dropped":    {"Café Newsletter", "caf-newsletter"},
		"non-latin dropped":  {"Информация", "form"},
		"nothing usable":     {"!!!", "form"},
		"empty":              {"", "form"},
		"only spaces":        {"   ", "form"},
	} {
		if got := slugify(tc.in); got != tc.want {
			t.Errorf("%s: slugify(%q) = %q, want %q", name, tc.in, got, tc.want)
		}
	}
}

// A slug that isn't URL-safe would break the storefront route it's resolved through
func TestSlugifyAlwaysProducesAUsableSlug(t *testing.T) {
	for _, in := range []string{"", "   ", "!!!", "Newsletter: 10% Off!", "Информация", "a b c", "---"} {
		got := slugify(in)
		if got == "" {
			t.Errorf("slugify(%q) = empty, want a usable fallback", in)
			continue
		}
		for _, r := range got {
			if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
				t.Errorf("slugify(%q) = %q, which contains the unsafe rune %q", in, got, r)
				break
			}
		}
		if got[0] == '-' || got[len(got)-1] == '-' {
			t.Errorf("slugify(%q) = %q, want no leading or trailing dash", in, got)
		}
	}
}

func TestSlugifyIsIdempotent(t *testing.T) {
	for _, in := range []string{"Newsletter Signup", "!!!", "Form 2026", "a   ---   b"} {
		once := slugify(in)
		if twice := slugify(once); twice != once {
			t.Errorf("slugify(slugify(%q)) = %q, want %q", in, twice, once)
		}
	}
}

// Status and type come off the admin form as free strings, and an unrecognized one has to land on
// the safe default rather than a value the storefront can't render
func TestNormStatusDefaultsToDraft(t *testing.T) {
	if got := normStatus("active"); got != "active" {
		t.Errorf("normStatus(active) = %q, want active", got)
	}
	for _, in := range []string{"draft", "", "ACTIVE", "Active", "live", "published", " active "} {
		if got := normStatus(in); got != "draft" {
			t.Errorf("normStatus(%q) = %q, want draft - only an exact \"active\" publishes a form", in, got)
		}
	}
}

func TestNormTypeDefaultsToPopup(t *testing.T) {
	for _, in := range []string{"inline", "flyout", "popup"} {
		if got := normType(in); got != in {
			t.Errorf("normType(%q) = %q, want it kept", in, got)
		}
	}
	for _, in := range []string{"", "modal", "POPUP", "Inline", "banner", " inline "} {
		if got := normType(in); got != "popup" {
			t.Errorf("normType(%q) = %q, want the popup fallback", in, got)
		}
	}
}

// Blank admin fields fall back to the built-in copy, so a form never renders with an empty button
func TestOrDefault(t *testing.T) {
	for name, tc := range map[string]struct{ in, def, want string }{
		"value wins":   {"Subscribe", "Sign up", "Subscribe"},
		"empty":        {"", "Sign up", "Sign up"},
		"whitespace":   {"   ", "Sign up", "Sign up"},
		"tab only":     {"\t\n", "Sign up", "Sign up"},
		"padded value": {"  Subscribe  ", "Sign up", "  Subscribe  "}, // kept verbatim, trimming is only the emptiness test
	} {
		if got := orDefault(tc.in, tc.def); got != tc.want {
			t.Errorf("%s: orDefault(%q, %q) = %q, want %q", name, tc.in, tc.def, got, tc.want)
		}
	}
}

// A nil or zero uuid has to become a NULL column rather than the all-zeros uuid, which would be a
// foreign key pointing at nothing
func TestNullUUIDTreatsTheZeroValueAsUnset(t *testing.T) {
	id := uuid.New()
	nilID := uuid.Nil

	if got := sqlnull.UUIDNonZero(nil); got.Valid {
		t.Error("nullUUID(nil) is Valid, want NULL")
	}
	if got := sqlnull.UUIDNonZero(&nilID); got.Valid {
		t.Error("nullUUID(&uuid.Nil) is Valid, want NULL rather than a zero foreign key")
	}
	got := sqlnull.UUIDNonZero(&id)
	if !got.Valid || got.UUID != id {
		t.Errorf("nullUUID(&id) = %v, want the id", got)
	}

	if got := sqlnull.UUIDValueNonZero(uuid.Nil); got.Valid {
		t.Error("nullUUIDv(uuid.Nil) is Valid, want NULL")
	}
	if got := sqlnull.UUIDValueNonZero(id); !got.Valid || got.UUID != id {
		t.Errorf("nullUUIDv(id) = %v, want the id", got)
	}
}
