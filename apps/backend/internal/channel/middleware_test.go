package channel_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/locale"
)

var (
	usID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	euID  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	b2bID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
)

func active(id uuid.UUID, slug, publicURL string, isDefault bool) channel.Channel {
	return channel.Channel{ID: id, Slug: slug, PublicURL: publicURL, IsActive: true, IsDefault: isDefault}
}

func inactive(id uuid.UUID, slug, publicURL string, isDefault bool) channel.Channel {
	ch := active(id, slug, publicURL, isDefault)
	ch.IsActive = false
	return ch
}

// Mirrors GetChannelByStorefrontHost's regex - host portion of public_url, scheme optional
var hostRe = regexp.MustCompile(`^(?:https?://)?([^/]+)`)

type fakeRepo struct{ channels []channel.Channel }

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (channel.Channel, error) {
	for _, ch := range f.channels {
		if ch.ID == id {
			return ch, nil
		}
	}
	return channel.Channel{}, channel.ErrChannelNotFound
}

func (f *fakeRepo) GetBySlug(_ context.Context, slug string) (channel.Channel, error) {
	for _, ch := range f.channels {
		if ch.Slug == slug {
			return ch, nil
		}
	}
	return channel.Channel{}, channel.ErrChannelNotFound
}

func (f *fakeRepo) GetByStorefrontHost(_ context.Context, host string) (channel.Channel, error) {
	for _, ch := range f.channels {
		if ch.PublicURL == "" {
			continue
		}
		if m := hostRe.FindStringSubmatch(ch.PublicURL); m != nil && m[1] == host {
			return ch, nil
		}
	}
	return channel.Channel{}, channel.ErrChannelNotFound
}

func (f *fakeRepo) GetDefault(_ context.Context) (channel.Channel, error) {
	for _, ch := range f.channels {
		if ch.IsDefault {
			return ch, nil
		}
	}
	return channel.Channel{}, channel.ErrChannelNotFound
}

func (f *fakeRepo) ListLocales(_ context.Context, _ uuid.UUID) ([]locale.Locale, error) {
	return nil, nil
}

func (f *fakeRepo) Create(context.Context, channel.CreateChannelInput) (channel.Channel, error) {
	panic("not used")
}
func (f *fakeRepo) List(context.Context) ([]channel.Channel, error) { panic("not used") }
func (f *fakeRepo) ListByIDs(context.Context, []uuid.UUID) (map[uuid.UUID]channel.Channel, error) {
	panic("not used")
}
func (f *fakeRepo) Update(context.Context, channel.UpdateChannelInput) (channel.Channel, error) {
	panic("not used")
}
func (f *fakeRepo) Delete(context.Context, uuid.UUID) error { panic("not used") }
func (f *fakeRepo) SetDefault(context.Context, uuid.UUID) (channel.Channel, error) {
	panic("not used")
}
func (f *fakeRepo) ListLocalesByStoreIDs(context.Context, []uuid.UUID) (map[uuid.UUID][]locale.Locale, error) {
	panic("not used")
}
func (f *fakeRepo) SetLocales(context.Context, uuid.UUID, []string) error { panic("not used") }

// Runs one request through the given middleware and reports what landed in context
func resolveVia(t *testing.T, mw func(http.Handler) http.Handler, mutate func(*http.Request)) (*channel.Channel, bool, int) {
	t.Helper()
	var got *channel.Channel
	var ok bool
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok = channel.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://api.internal/api/front/graphql", nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return got, ok, rec.Code
}

func TestMiddlewareResolution(t *testing.T) {
	stores := []channel.Channel{
		active(usID, "us", "https://shop.example.com", true),
		active(euID, "eu", "https://shop.example.eu", false),
		inactive(b2bID, "b2b", "https://b2b.example.com", false),
	}
	mw := channel.Middleware(channel.NewService(&fakeRepo{channels: stores}, nil))

	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   uuid.UUID // uuid.Nil = expect no channel in context
	}{
		{"valid id resolves that channel", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", euID.String())
		}, euID},
		{"id header wins over matching host", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", euID.String())
			r.Host = "shop.example.com"
		}, euID},
		// A retired or pre-launch channel is a deliberate operator state - its storefront goes dark rather than quietly serving a different market
		{"inactive channel id resolves nothing", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", b2bID.String())
		}, uuid.Nil},
		// The stale-id case: a config copied between environments must fail loudly, not serve the default
		{"unknown id resolves nothing", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", uuid.MustParse("44444444-4444-4444-4444-444444444444").String())
		}, uuid.Nil},
		{"malformed id resolves nothing", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", "eu")
		}, uuid.Nil},
		{"slug header is not honored", func(r *http.Request) {
			r.Header.Set("X-Store-Slug", "eu")
		}, usID},
		{"host matches public_url", func(r *http.Request) {
			r.Host = "shop.example.eu"
		}, euID},
		{"host match strips port and folds case", func(r *http.Request) {
			r.Host = "Shop.Example.EU:443"
		}, euID},
		{"inactive host match falls through to default", func(r *http.Request) {
			r.Host = "b2b.example.com"
		}, usID},
		{"nothing matches resolves the default", nil, usID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, code := resolveVia(t, mw, tc.mutate)
			if code != http.StatusOK {
				t.Fatalf("request failed with %d - middleware must never gate requests", code)
			}
			if tc.want == uuid.Nil {
				if ok {
					t.Fatalf("expected no channel, got %q", got.Slug)
				}
				return
			}
			if !ok || got == nil {
				t.Fatalf("expected channel %s, got none", tc.want)
			}
			if got.ID != tc.want {
				t.Fatalf("expected channel %s, got %s (%q)", tc.want, got.ID, got.Slug)
			}
		})
	}
}

func TestMiddlewareNoDefault(t *testing.T) {
	stores := []channel.Channel{
		active(euID, "eu", "https://shop.example.eu", false),
		inactive(b2bID, "b2b", "https://b2b.example.com", false),
	}
	mw := channel.Middleware(channel.NewService(&fakeRepo{channels: stores}, nil))

	cases := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{"no signals at all", nil},
		{"inactive channel id resolves nothing", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", b2bID.String())
		}},
		{"inactive host resolves nothing", func(r *http.Request) {
			r.Host = "b2b.example.com"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, code := resolveVia(t, mw, tc.mutate)
			if code != http.StatusOK {
				t.Fatalf("request failed with %d - middleware must never gate requests", code)
			}
			if ok {
				t.Fatalf("expected no channel, got %q", got.Slug)
			}
		})
	}
}

func TestMiddlewareInactiveDefaultResolvesNothing(t *testing.T) {
	stores := []channel.Channel{inactive(usID, "us", "https://shop.example.com", true)}
	mw := channel.Middleware(channel.NewService(&fakeRepo{channels: stores}, nil))
	got, ok, code := resolveVia(t, mw, nil)
	if code != http.StatusOK {
		t.Fatalf("request failed with %d", code)
	}
	if ok {
		t.Fatalf("inactive default must not resolve, got %q", got.Slug)
	}
}

func TestAdminMiddleware(t *testing.T) {
	stores := []channel.Channel{
		active(usID, "us", "https://shop.example.com", true),
		active(euID, "eu", "https://shop.example.eu", false),
		inactive(b2bID, "b2b", "https://b2b.example.com", false),
	}
	mw := channel.AdminMiddleware(channel.NewService(&fakeRepo{channels: stores}, nil))

	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   uuid.UUID
	}{
		// No header = all channels, NOT the default - conflating the two is what silently scopes a Global admin to one store
		{"no header means no scope", nil, uuid.Nil},
		{"host never scopes the admin surface", func(r *http.Request) {
			r.Host = "shop.example.eu"
		}, uuid.Nil},
		{"resolves by id", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", euID.String())
		}, euID},
		{"slug parity for tooling", func(r *http.Request) {
			r.Header.Set("X-Store-Slug", "eu")
		}, euID},
		{"malformed id yields no scope, no fallback", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", "not-a-uuid")
			r.Header.Set("X-Store-Slug", "eu")
		}, uuid.Nil},
		// Admins may scope to inactive channels - deliberate asymmetry with the front surface
		{"inactive channel is scopeable", func(r *http.Request) {
			r.Header.Set("X-Channel-Id", b2bID.String())
		}, b2bID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, code := resolveVia(t, mw, tc.mutate)
			if code != http.StatusOK {
				t.Fatalf("request failed with %d", code)
			}
			if tc.want == uuid.Nil {
				if ok {
					t.Fatalf("expected no scope, got %q", got.Slug)
				}
				return
			}
			if !ok || got == nil {
				t.Fatalf("expected channel %s, got none", tc.want)
			}
			if got.ID != tc.want {
				t.Fatalf("expected channel %s, got %s (%q)", tc.want, got.ID, got.Slug)
			}
		})
	}
}
