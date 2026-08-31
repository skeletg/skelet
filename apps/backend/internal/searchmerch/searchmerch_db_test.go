package searchmerch

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
)

// Merchandising sits on the hot search path and every lookup is fail-soft, which is exactly what makes
// it easy to break quietly: a synonym group that stops expanding, a pin that comes back out of order or
// a redirect keyed on the wrong casing all degrade search results with nothing in the logs
//
// DB-backed because the scope rule (global groups apply everywhere, another channel's never do), the
// ordering and the upsert conflict targets are all the queries'

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	conn := testdb.Require(t)
	return NewService(dbsqlc.New(conn)), conn
}

// Two distinct channels, so cross-channel leakage is observable rather than assumed
func seedChannels(t *testing.T, conn *sql.DB) (uuid.UUID, uuid.UUID) {
	t.Helper()
	rows, err := conn.Query(`SELECT id FROM channels ORDER BY created_at LIMIT 2`)
	if err != nil {
		t.Skipf("no channels to scope against: %v", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan channel: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read channels: %v", err)
	}
	if len(ids) < 2 {
		t.Skip("need two channels to prove per-channel scoping")
	}
	return ids[0], ids[1]
}

func seedProduct(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	var typeID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM catalog_product_types LIMIT 1`).Scan(&typeID); err != nil {
		t.Skipf("no product type to hang a fixture product off: %v", err)
	}
	suffix := uuid.NewString()[:8]
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO catalog_products (product_type_id, sku, url, kind)
		 VALUES ($1, $2, $3, 'simple') RETURNING id`,
		typeID, "MERCH-"+suffix, "merch-"+suffix).Scan(&id); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, id) }) // pins and boosts cascade
	return id
}

// A synonym group in the given scope - nil channel is the global group every channel sees
func seedSynonym(t *testing.T, conn *sql.DB, channelID *uuid.UUID, terms string) {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO search_synonyms (channel_id, terms) VALUES ($1, $2) RETURNING id`,
		nullableUUID(channelID), terms).Scan(&id); err != nil {
		t.Fatalf("seed synonym: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM search_synonyms WHERE id = $1`, id) })
}

// Nonsense stems, so a group seeded here can never be matched by another test's terms
func stems(t *testing.T) (string, string, string) {
	t.Helper()
	base := "zq" + strings.ToLower(uuid.NewString()[:6])
	return base + "a", base + "b", base + "c"
}

func TestNormalizeIsTheQueryKey(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"mixed case":         {"Winter Tyres", "winter tyres"},
		"padded":             {"  winter tyres  ", "winter tyres"},
		"collapsed runs":     {"winter\t\n  tyres", "winter tyres"},
		"already normalized": {"winter tyres", "winter tyres"},
		"blank":              {"   ", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Normalize(tc.in); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitTermsDedupesAndLowercases(t *testing.T) { // operators paste these by hand, trailing commas and all
	for name, tc := range map[string]struct {
		in   string
		want []string
	}{
		"padded list":     {"tyre, tire , tires", []string{"tyre", "tire", "tires"}},
		"mixed case":      {"Tyre, TIRE", []string{"tyre", "tire"}},
		"repeated term":   {"tyre, tyre, tire", []string{"tyre", "tire"}},
		"trailing commas": {"tyre, tire,,", []string{"tyre", "tire"}},
		"one term":        {"tyre", []string{"tyre"}},
		"nothing at all":  {" , ", nil},
	} {
		t.Run(name, func(t *testing.T) {
			got := splitTerms(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("splitTerms(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("splitTerms(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestExpandQueryAppendsTheRestOfAMatchedGroup(t *testing.T) {
	// Synonyms expand at query time precisely so operators don't need a reindex - if this stops firing
	// the group looks configured in the dashboard and changes nothing about the results
	svc, conn := newTestService(t)
	ctx := context.Background()
	channelID, _ := seedChannels(t, conn)
	tyre, tire, tires := stems(t)
	seedSynonym(t, conn, &channelID, tyre+", "+tire+", "+tires)

	for name, tc := range map[string]struct{ query, want string }{
		"a term in the group":          {tyre + " rack", tyre + " rack " + tire + " " + tires},
		"another term in the group":    {tires, tires + " " + tyre + " " + tire},
		"casing does not stop a match": {strings.ToUpper(tyre), strings.ToUpper(tyre) + " " + tire + " " + tires},
		"already holds a group member": {tyre + " " + tire, tyre + " " + tire + " " + tires},
		"the whole group":              {tyre + " " + tire + " " + tires, tyre + " " + tire + " " + tires},
		"nothing in the group":         {"brake pads", "brake pads"},
		"an empty query":               {"", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := svc.ExpandQuery(ctx, channelID, tc.query); got != tc.want {
				t.Errorf("ExpandQuery(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestGlobalGroupsApplyEverywhereAndChannelGroupsDoNot(t *testing.T) {
	// One catalogue shared across storefronts wants global groups, while a channel's own vocabulary
	// must not bleed - expanding another channel's group silently returns products it never listed for
	svc, conn := newTestService(t)
	ctx := context.Background()
	mine, theirs := seedChannels(t, conn)
	shared, sharedAlt, _ := stems(t)
	local, localAlt, _ := stems(t)

	seedSynonym(t, conn, nil, shared+", "+sharedAlt)
	seedSynonym(t, conn, &theirs, local+", "+localAlt)

	if got, want := svc.ExpandQuery(ctx, mine, shared), shared+" "+sharedAlt; got != want {
		t.Errorf("global group on my channel = %q, want %q", got, want)
	}
	if got := svc.ExpandQuery(ctx, mine, local); got != local {
		t.Errorf("another channel's group leaked: ExpandQuery = %q, want %q untouched", got, local)
	}
	if got, want := svc.ExpandQuery(ctx, theirs, local), local+" "+localAlt; got != want {
		t.Errorf("own-channel group on its channel = %q, want %q", got, want)
	}
}

func TestAddSynonymNeedsTwoDistinctTerms(t *testing.T) {
	// A one-term group expands to nothing, so accepting it leaves a row that looks live in the
	// dashboard and can never fire
	svc, conn := newTestService(t)
	ctx := context.Background()
	channelID, _ := seedChannels(t, conn)
	a, b, _ := stems(t)

	for name, tc := range map[string]struct {
		terms   string
		wantErr bool
	}{
		"two terms":            {a + ", " + b, false},
		"one term":             {a, true},
		"the same term twice":  {a + ", " + a, true},
		"one term and a comma": {a + ",", true},
		"nothing":              {" , ", true},
	} {
		t.Run(name, func(t *testing.T) {
			syn, err := svc.AddSynonym(ctx, &channelID, tc.terms)
			if tc.wantErr != (err != nil) {
				t.Fatalf("AddSynonym(%q) err = %v, wantErr %v", tc.terms, err, tc.wantErr)
			}
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = svc.DeleteSynonym(ctx, channelID, syn.ID) })
			if syn.Terms != a+", "+b {
				t.Errorf("stored terms = %q, want the normalized list", syn.Terms)
			}
			if syn.ChannelID == nil || *syn.ChannelID != channelID {
				t.Errorf("stored scope = %v, want channel %v", syn.ChannelID, channelID)
			}
		})
	}
}

func TestRedirectsAnswerOnTheNormalizedQueryOnly(t *testing.T) {
	// The shopper's raw input carries their casing and spacing - matching literally means the redirect
	// fires for some spellings and silently not for others
	svc, conn := newTestService(t)
	ctx := context.Background()
	mine, theirs := seedChannels(t, conn)
	query := "zq" + strings.ToLower(uuid.NewString()[:6])

	red, err := svc.UpsertRedirect(ctx, mine, "  "+strings.ToUpper(query)+"  ", "/pages/faq")
	if err != nil {
		t.Fatalf("UpsertRedirect: %v", err)
	}
	t.Cleanup(func() { _ = svc.DeleteRedirect(ctx, mine, red.ID) })
	if red.Query != query {
		t.Errorf("stored query = %q, want the normalized %q", red.Query, query)
	}

	for name, spelling := range map[string]string{
		"as typed":   query,
		"upper case": strings.ToUpper(query),
		"padded":     "  " + query + " ",
	} {
		t.Run(name, func(t *testing.T) {
			target, ok := svc.RedirectFor(ctx, mine, spelling)
			if !ok || target != "/pages/faq" {
				t.Errorf("RedirectFor(%q) = (%q, %v), want (\"/pages/faq\", true)", spelling, target, ok)
			}
		})
	}
	if _, ok := svc.RedirectFor(ctx, mine, query+" tyres"); ok {
		t.Error("a longer query matched a redirect keyed on the short one")
	}
	if _, ok := svc.RedirectFor(ctx, theirs, query); ok {
		t.Error("another channel's redirect fired")
	}
}

func TestUpsertRedirectRejectsTargetsTheBrowserCannotFollow(t *testing.T) {
	svc, conn := newTestService(t)
	ctx := context.Background()
	channelID, _ := seedChannels(t, conn)

	for name, tc := range map[string]struct {
		query, target string
		wantErr       bool
	}{
		"a site path":      {"zqfaq1", "/pages/faq", false},
		"an absolute url":  {"zqfaq2", "https://help.example.com", false},
		"a bare word":      {"zqfaq3", "pages/faq", true},
		"a scheme we skip": {"zqfaq4", "javascript:alert(1)", true},
		"no target":        {"zqfaq5", "   ", true},
		"no query":         {"   ", "/pages/faq", true},
	} {
		t.Run(name, func(t *testing.T) {
			red, err := svc.UpsertRedirect(ctx, channelID, tc.query, tc.target)
			if tc.wantErr != (err != nil) {
				t.Fatalf("UpsertRedirect(%q, %q) err = %v, wantErr %v", tc.query, tc.target, err, tc.wantErr)
			}
			if err == nil {
				t.Cleanup(func() { _ = svc.DeleteRedirect(ctx, channelID, red.ID) })
			}
		})
	}
}

func TestPinsComeBackInTheOrderTheOperatorSet(t *testing.T) {
	// A pin's whole point is the forced slot - out of order it becomes an arbitrary reshuffle of the
	// top results, which reads as a broken relevance model rather than a merchandising mistake
	svc, conn := newTestService(t)
	ctx := context.Background()
	channelID, theirs := seedChannels(t, conn)
	query := "zq" + strings.ToLower(uuid.NewString()[:6])
	third, first, second := seedProduct(t, conn), seedProduct(t, conn), seedProduct(t, conn)

	for pos, productID := range map[int32]uuid.UUID{3: third, 1: first, 2: second} {
		pin, err := svc.UpsertPin(ctx, channelID, strings.ToUpper(query), productID, pos)
		if err != nil {
			t.Fatalf("UpsertPin: %v", err)
		}
		t.Cleanup(func() { _ = svc.DeletePin(ctx, channelID, pin.ID) })
		if pin.Query != query {
			t.Errorf("pin stored query %q, want the normalized %q", pin.Query, query)
		}
	}

	got := svc.PinsFor(ctx, channelID, query)
	want := []uuid.UUID{first, second, third}
	if len(got) != len(want) {
		t.Fatalf("PinsFor = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d = %v, want %v", i+1, got[i], want[i])
		}
	}
	if pins := svc.PinsFor(ctx, theirs, query); len(pins) != 0 {
		t.Errorf("another channel's pins = %v, want none", pins)
	}
	if pins := svc.PinsFor(ctx, channelID, "zqnothingpinned"); len(pins) != 0 {
		t.Errorf("unpinned query returned %v", pins)
	}
}

func TestUpsertPinClampsAndRepositionsInPlace(t *testing.T) {
	// Position is 1-based in the UI, so a 0 or negative value is an unset field rather than "before the
	// first result" - and re-pinning has to move the existing row, not add a second slot
	svc, conn := newTestService(t)
	ctx := context.Background()
	channelID, _ := seedChannels(t, conn)
	productID := seedProduct(t, conn)
	query := "zq" + strings.ToLower(uuid.NewString()[:6])

	// One cleanup for the parent - every upsert below targets the same conflict key, so they all
	// resolve to one row and per-subtest deletes would drop it before the count below
	var pinID uuid.UUID
	t.Cleanup(func() { _ = svc.DeletePin(ctx, channelID, pinID) })

	for name, tc := range map[string]struct{ given, want int32 }{
		"unset":    {0, 1},
		"negative": {-4, 1},
		"explicit": {2, 2},
	} {
		t.Run(name, func(t *testing.T) {
			pin, err := svc.UpsertPin(ctx, channelID, query, productID, tc.given)
			if err != nil {
				t.Fatalf("UpsertPin: %v", err)
			}
			pinID = pin.ID
			if pin.Position != tc.want {
				t.Errorf("position = %d, want %d", pin.Position, tc.want)
			}
		})
	}
	if pins := svc.PinsFor(ctx, channelID, query); len(pins) != 1 {
		t.Errorf("re-pinning left %d rows, want the one moved in place", len(pins))
	}
	if _, err := svc.UpsertPin(ctx, channelID, "   ", productID, 1); err == nil {
		t.Error("an empty query was pinned; it would match every blank search")
	}
}

func TestBoostsAreKeyedOnWhatTheShopperTyped(t *testing.T) {
	// Merchandising matches the typed query, not the synonym-expanded one - keying on the expansion
	// would apply a boost meant for "tyres" to every query that expanded to include it
	svc, conn := newTestService(t)
	ctx := context.Background()
	channelID, theirs := seedChannels(t, conn)
	lifted, buried := seedProduct(t, conn), seedProduct(t, conn)
	query := "zq" + strings.ToLower(uuid.NewString()[:6])

	for productID, weight := range map[uuid.UUID]float64{lifted: 2.5, buried: -1.5} {
		boost, err := svc.UpsertBoost(ctx, channelID, strings.ToUpper(query), productID, weight)
		if err != nil {
			t.Fatalf("UpsertBoost: %v", err)
		}
		t.Cleanup(func() { _ = svc.DeleteBoost(ctx, channelID, boost.ID) })
	}

	got := svc.BoostsFor(ctx, channelID, "  "+query+"  ")
	if len(got) != 2 || got[lifted] != 2.5 || got[buried] != -1.5 {
		t.Errorf("BoostsFor = %v, want the lift and the bury", got)
	}
	if expanded := svc.BoostsFor(ctx, channelID, query+" "+query); len(expanded) != 0 {
		t.Errorf("an expanded query picked up boosts: %v", expanded)
	}
	if other := svc.BoostsFor(ctx, theirs, query); len(other) != 0 {
		t.Errorf("another channel's boosts = %v, want none", other)
	}
}

func TestUpsertBoostRefusesAZeroWeight(t *testing.T) {
	// Zero neither lifts nor buries, so storing it is a row the operator believes is doing something
	svc, conn := newTestService(t)
	ctx := context.Background()
	channelID, _ := seedChannels(t, conn)
	productID := seedProduct(t, conn)

	for name, tc := range map[string]struct {
		weight  float64
		wantErr bool
	}{
		"a lift": {3, false},
		"a bury": {-3, false},
		"zero":   {0, true},
	} {
		t.Run(name, func(t *testing.T) {
			boost, err := svc.UpsertBoost(ctx, channelID, "zqweight", productID, tc.weight)
			if tc.wantErr != (err != nil) {
				t.Fatalf("UpsertBoost(%v) err = %v, wantErr %v", tc.weight, err, tc.wantErr)
			}
			if err == nil {
				t.Cleanup(func() { _ = svc.DeleteBoost(ctx, channelID, boost.ID) })
				if boost.Weight != tc.weight {
					t.Errorf("weight = %v, want %v", boost.Weight, tc.weight)
				}
			}
		})
	}
}

func contains(ids []uuid.UUID, want uuid.UUID) bool {
	return slices.Contains(ids, want)
}

// The delete resolvers gate on a client-supplied channelId but address the row by id, so the query has
// to bind BOTH or an admin scoped to one channel can delete another channel's merchandising by id. Each
// case deletes from the WRONG channel first (a no-op that must leave the row intact) then the owner's.
func TestDeletesAreScopedToTheOwningChannel(t *testing.T) {
	svc, conn := newTestService(t)
	ctx := context.Background()
	mine, theirs := seedChannels(t, conn)
	productID := seedProduct(t, conn)
	a, b, _ := stems(t)

	t.Run("synonym", func(t *testing.T) {
		syn, err := svc.AddSynonym(ctx, &mine, a+", "+b)
		if err != nil {
			t.Fatalf("add synonym: %v", err)
		}
		t.Cleanup(func() { _ = svc.DeleteSynonym(ctx, mine, syn.ID) })
		present := func() bool {
			rows, err := svc.ListSynonyms(ctx, mine)
			if err != nil {
				t.Fatalf("list synonyms: %v", err)
			}
			ids := make([]uuid.UUID, 0, len(rows))
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			return contains(ids, syn.ID)
		}
		if err := svc.DeleteSynonym(ctx, theirs, syn.ID); err != nil {
			t.Fatalf("cross-channel delete errored: %v", err)
		}
		if !present() {
			t.Fatal("another channel deleted my synonym")
		}
		if err := svc.DeleteSynonym(ctx, mine, syn.ID); err != nil {
			t.Fatalf("owner delete: %v", err)
		}
		if present() {
			t.Fatal("owner delete left the synonym behind")
		}
	})

	t.Run("pin", func(t *testing.T) {
		pin, err := svc.UpsertPin(ctx, mine, a, productID, 1)
		if err != nil {
			t.Fatalf("upsert pin: %v", err)
		}
		t.Cleanup(func() { _ = svc.DeletePin(ctx, mine, pin.ID) })
		present := func() bool {
			rows, err := svc.ListPins(ctx, mine)
			if err != nil {
				t.Fatalf("list pins: %v", err)
			}
			ids := make([]uuid.UUID, 0, len(rows))
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			return contains(ids, pin.ID)
		}
		if err := svc.DeletePin(ctx, theirs, pin.ID); err != nil {
			t.Fatalf("cross-channel delete errored: %v", err)
		}
		if !present() {
			t.Fatal("another channel deleted my pin")
		}
		if err := svc.DeletePin(ctx, mine, pin.ID); err != nil {
			t.Fatalf("owner delete: %v", err)
		}
		if present() {
			t.Fatal("owner delete left the pin behind")
		}
	})

	t.Run("boost", func(t *testing.T) {
		boost, err := svc.UpsertBoost(ctx, mine, a, productID, 2)
		if err != nil {
			t.Fatalf("upsert boost: %v", err)
		}
		t.Cleanup(func() { _ = svc.DeleteBoost(ctx, mine, boost.ID) })
		present := func() bool {
			rows, err := svc.ListBoosts(ctx, mine)
			if err != nil {
				t.Fatalf("list boosts: %v", err)
			}
			ids := make([]uuid.UUID, 0, len(rows))
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			return contains(ids, boost.ID)
		}
		if err := svc.DeleteBoost(ctx, theirs, boost.ID); err != nil {
			t.Fatalf("cross-channel delete errored: %v", err)
		}
		if !present() {
			t.Fatal("another channel deleted my boost")
		}
		if err := svc.DeleteBoost(ctx, mine, boost.ID); err != nil {
			t.Fatalf("owner delete: %v", err)
		}
		if present() {
			t.Fatal("owner delete left the boost behind")
		}
	})

	t.Run("redirect", func(t *testing.T) {
		red, err := svc.UpsertRedirect(ctx, mine, a, "/pages/faq")
		if err != nil {
			t.Fatalf("upsert redirect: %v", err)
		}
		t.Cleanup(func() { _ = svc.DeleteRedirect(ctx, mine, red.ID) })
		present := func() bool {
			rows, err := svc.ListRedirects(ctx, mine)
			if err != nil {
				t.Fatalf("list redirects: %v", err)
			}
			ids := make([]uuid.UUID, 0, len(rows))
			for _, r := range rows {
				ids = append(ids, r.ID)
			}
			return contains(ids, red.ID)
		}
		if err := svc.DeleteRedirect(ctx, theirs, red.ID); err != nil {
			t.Fatalf("cross-channel delete errored: %v", err)
		}
		if !present() {
			t.Fatal("another channel deleted my redirect")
		}
		if err := svc.DeleteRedirect(ctx, mine, red.ID); err != nil {
			t.Fatalf("owner delete: %v", err)
		}
		if present() {
			t.Fatal("owner delete left the redirect behind")
		}
	})
}
