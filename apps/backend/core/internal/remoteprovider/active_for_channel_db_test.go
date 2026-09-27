package remoteprovider

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The request cache answers every kind from this read, so it must match ListActiveByKind per kind
func TestTheChannelReadCarriesEveryActiveKindItCanSee(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	a, b := uuid.New(), uuid.New()
	for _, ch := range []uuid.UUID{a, b} {
		if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code, is_active) VALUES ($1,$2,'Scope','USD',true)`, ch, "scope-"+ch.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	prefix := "scope-" + uuid.NewString()[:8]
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM remote_providers WHERE slug LIKE $1`, prefix+"%")
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = ANY($1)`, "{"+a.String()+","+b.String()+"}")
	})
	mk := func(kind Kind, name string, ch *uuid.UUID, active bool) {
		p, err := repo.Create(ctx, CreateInput{Kind: kind, Slug: prefix + "-" + name, Name: name, TargetURL: "https://p.example.test", Config: []byte("{}"), TimeoutMs: 1000, FailurePolicy: FailOpen, ChannelID: ch}, "secret")
		if err != nil {
			t.Fatal(err)
		}
		if !active {
			if _, err := repo.SetActive(ctx, p.ID, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk(KindSearch, "a-search", &a, true)
	mk(KindPricing, "global-pricing", nil, true)
	mk(KindPricing, "b-pricing", &b, true)
	mk(KindTaxProvider, "a-tax-off", &a, false)

	names := func(ch *uuid.UUID) []string {
		ps, err := repo.ListActiveForChannel(ctx, ch)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, p := range ps {
			if strings.HasPrefix(p.Slug, prefix) {
				out = append(out, p.Name)
			}
		}
		return out
	}
	if got := strings.Join(names(&a), ","); got != "global-pricing,a-search" {
		t.Fatalf("channel a sees %q, want the global pricing and its own search in kind order - never b's or an inactive one", got)
	}
	if got := strings.Join(names(nil), ","); got != "global-pricing" {
		t.Fatalf("no channel sees %q, want only global providers", got)
	}
	for _, kind := range []Kind{KindPricing, KindSearch, KindTaxProvider} {
		byKind, err := repo.ListActiveByKind(ctx, kind, &a)
		if err != nil {
			t.Fatal(err)
		}
		all, _ := repo.ListActiveForChannel(ctx, &a)
		var fromAll []uuid.UUID
		for _, p := range all {
			if p.Kind == kind {
				fromAll = append(fromAll, p.ID)
			}
		}
		if len(fromAll) != len(byKind) {
			t.Fatalf("%s: the channel read holds %d providers, the per-kind read %d", kind, len(fromAll), len(byKind))
		}
		for i := range byKind {
			if byKind[i].ID != fromAll[i] {
				t.Fatalf("%s: the channel read reorders providers the per-kind read returns", kind)
			}
		}
	}
}
