package channel_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/locale"
)

func loc(code string, isActive bool) locale.Locale {
	return locale.Locale{Code: code, IsActive: isActive}
}

func TestClampLocale(t *testing.T) {
	store := channel.Channel{
		DefaultLocale: "en",
		Locales:       []locale.Locale{loc("en", true), loc("de", true), loc("fr", false)},
	}

	cases := []struct {
		name      string
		sv        channel.Channel
		requested string
		want      string
	}{
		{"supported locale passes through", store, "de", "de"},
		{"case folds to the canonical stored code", store, "DE", "de"},
		{"unsupported locale clamps to default", store, "sr", "en"},
		{"globally deactivated locale clamps to default", store, "fr", "en"},
		{"empty request clamps to default", store, "", "en"},
		{"whitespace-only request clamps to default", store, "  ", "en"},
		{
			"deactivated default still serves - the store must not go dark",
			channel.Channel{DefaultLocale: "en", Locales: []locale.Locale{loc("en", false), loc("de", true)}},
			"en", "en",
		},
		{
			"no hydrated locales falls back to default match",
			channel.Channel{DefaultLocale: "en"},
			"EN", "en",
		},
		{
			"no default and no match passes the request through",
			channel.Channel{},
			"de", "de",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := channel.ClampLocale(tc.sv, tc.requested); got != tc.want {
				t.Fatalf("ClampLocale(%q) = %q, want %q", tc.requested, got, tc.want)
			}
		})
	}
}

func TestLocaleClampMiddleware(t *testing.T) {
	store := &channel.Channel{
		DefaultLocale: "en",
		Locales:       []locale.Locale{loc("en", true), loc("de", true)},
	}

	run := func(t *testing.T, withChannel bool, requested string) string {
		t.Helper()
		var seen string
		h := channel.LocaleClampMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = locale.FromContext(r.Context())
		}))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := locale.WithLocale(req.Context(), requested)
		if withChannel {
			ctx = channel.WithChannel(ctx, store)
		}
		h.ServeHTTP(httptest.NewRecorder(), req.WithContext(ctx))
		return seen
	}

	if got := run(t, true, "sr"); got != "en" {
		t.Fatalf("unsupported locale should clamp to channel default, got %q", got)
	}
	if got := run(t, true, "de"); got != "de" {
		t.Fatalf("supported locale should survive, got %q", got)
	}
	// Admin surface and probe contexts run without a channel - the request wins there
	if got := run(t, false, "sr"); got != "sr" {
		t.Fatalf("no channel means no clamping, got %q", got)
	}
}
