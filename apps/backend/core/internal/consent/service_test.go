package consent

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// fakeRepo is a hand-written in-memory Repository - Get returns a nil *StoredConsent for any (customer, kind) not explicitly stored,
// mirroring the real postgres repo's sql.ErrNoRows → nil contract that drives the service's default-fallback logic
type fakeRepo struct {
	stored    map[storeKey]StoredConsent
	events    []Event
	getErr    error
	upsertErr error
	listErr   error
	optIns    []EmailOptIn
}

type storeKey struct {
	customer uuid.UUID
	kind     string
}

func newFakeRepo() *fakeRepo { return &fakeRepo{stored: map[storeKey]StoredConsent{}} }

func (r *fakeRepo) put(customer uuid.UUID, kind string, optedIn bool, source string) {
	r.stored[storeKey{customer, kind}] = StoredConsent{
		CustomerID: customer,
		Kind:       kind,
		OptedIn:    optedIn,
		Source:     source,
		UpdatedAt:  time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func (r *fakeRepo) Upsert(_ context.Context, c Change) (StoredConsent, error) {
	if r.upsertErr != nil {
		return StoredConsent{}, r.upsertErr
	}
	r.put(c.CustomerID, c.Kind, c.OptedIn, c.Source)
	r.events = append(r.events, Event{ID: uuid.New(), CustomerID: c.CustomerID, Kind: c.Kind, OptedIn: c.OptedIn, Source: c.Source, AdminID: c.AdminID})
	return r.stored[storeKey{c.CustomerID, c.Kind}], nil
}

// The in-memory twin has no transaction to join, so it writes the same way
func (r *fakeRepo) UpsertTx(ctx context.Context, _ *sql.Tx, c Change) (StoredConsent, error) {
	return r.Upsert(ctx, c)
}

func (r *fakeRepo) Get(_ context.Context, customerID uuid.UUID, kind string) (*StoredConsent, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	sc, ok := r.stored[storeKey{customerID, kind}]
	if !ok {
		return nil, nil // no row → service uses Definition.DefaultOptIn
	}
	return &sc, nil
}

func (r *fakeRepo) ListByCustomer(_ context.Context, customerID uuid.UUID) ([]StoredConsent, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	var out []StoredConsent
	for k, v := range r.stored {
		if k.customer == customerID {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *fakeRepo) ListEvents(_ context.Context, customerID uuid.UUID, limit int32) ([]Event, error) {
	var out []Event
	for i := len(r.events) - 1; i >= 0 && int32(len(out)) < limit; i-- {
		if r.events[i].CustomerID == customerID {
			out = append(out, r.events[i])
		}
	}
	return out, nil
}

func (r *fakeRepo) RecordEmailOptIn(_ context.Context, o EmailOptIn) error {
	r.optIns = append(r.optIns, o)
	return nil
}

func (r *fakeRepo) ListEmailOptIns(_ context.Context, channelID uuid.UUID, email string) ([]EmailOptIn, error) {
	var out []EmailOptIn
	for _, o := range r.optIns {
		if o.ChannelID == channelID && o.Email == email {
			out = append(out, o)
		}
	}
	return out, nil
}

// The built-in kinds registered by defaults.go init() - Pinning these keeps
// the tests honest about the GDPR defaults the service relies on
const (
	kindMarketingEmail = "marketing_email"     // Required=false, DefaultOptIn=false
	kindTransactional  = "transactional_email" // Required=true,  DefaultOptIn=true
)

func TestGet_NoRowUsesDefaultOptIn(t *testing.T) {
	svc := NewService(newFakeRepo())
	cust := uuid.New()

	// Marketing: no row → GDPR default-deny
	c, err := svc.Get(context.Background(), cust, kindMarketingEmail)
	if err != nil {
		t.Fatalf("Get marketing: %v", err)
	}
	if !c.IsDefault {
		t.Error("expected IsDefault=true when no DB row exists")
	}
	if c.OptedIn {
		t.Error("marketing with no row must default to opted-out (GDPR)")
	}
	def, _ := Find(kindMarketingEmail)
	if c.OptedIn != def.DefaultOptIn {
		t.Errorf("OptedIn = %v, want def.DefaultOptIn %v", c.OptedIn, def.DefaultOptIn)
	}
	if c.Source != "" {
		t.Errorf("default consent Source = %q, want empty", c.Source)
	}

	// Transactional: no row → defaults opted-in (Required, DefaultOptIn=true)
	tc, err := svc.Get(context.Background(), cust, kindTransactional)
	if err != nil {
		t.Fatalf("Get transactional: %v", err)
	}
	if !tc.IsDefault || !tc.OptedIn {
		t.Errorf("transactional default: IsDefault=%v OptedIn=%v, want both true", tc.IsDefault, tc.OptedIn)
	}
}

func TestGet_StoredRowOverridesDefault(t *testing.T) {
	repo := newFakeRepo()
	cust := uuid.New()
	repo.put(cust, kindMarketingEmail, true, SourceProfile)
	svc := NewService(repo)

	c, err := svc.Get(context.Background(), cust, kindMarketingEmail)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c.IsDefault {
		t.Error("a stored row must set IsDefault=false")
	}
	if !c.OptedIn {
		t.Error("stored opted-in not reflected")
	}
	if c.Source != SourceProfile {
		t.Errorf("Source = %q, want %q", c.Source, SourceProfile)
	}
	if c.UpdatedAt.IsZero() {
		t.Error("stored row should carry UpdatedAt")
	}
}

func TestGet_UnknownKind(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.Get(context.Background(), uuid.New(), "does_not_exist")
	if !errors.Is(err, ErrUnknownConsent) {
		t.Fatalf("got %v, want ErrUnknownConsent", err)
	}
}

func TestGet_RepoErrorPropagates(t *testing.T) {
	boom := errors.New("db down")
	repo := newFakeRepo()
	repo.getErr = boom
	svc := NewService(repo)
	_, err := svc.Get(context.Background(), uuid.New(), kindMarketingEmail)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want underlying db error", err)
	}
}

func TestSetConsent_OptOutOfRequiredRejected(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.SetConsent(context.Background(), uuid.New(), kindTransactional, false, SourceProfile)
	if !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("got %v, want ErrConsentRequired", err)
	}
}

func TestSetConsent_OptInToRequiredAllowed(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	cust := uuid.New()
	c, err := svc.SetConsent(context.Background(), cust, kindTransactional, true, SourceAdmin)
	if err != nil {
		t.Fatalf("SetConsent: %v", err)
	}
	if c.IsDefault {
		t.Error("a SetConsent result is never a default")
	}
	if !c.OptedIn || c.Source != SourceAdmin {
		t.Errorf("bad consent: %+v", c)
	}
	if _, ok := repo.stored[storeKey{cust, kindTransactional}]; !ok {
		t.Error("SetConsent did not persist a row")
	}
}

func TestSetConsent_UnknownKind(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.SetConsent(context.Background(), uuid.New(), "nope", true, SourceProfile)
	if !errors.Is(err, ErrUnknownConsent) {
		t.Fatalf("got %v, want ErrUnknownConsent", err)
	}
}

func TestSetConsent_OptOutOfMarketingAllowed(t *testing.T) {
	svc := NewService(newFakeRepo())
	c, err := svc.SetConsent(context.Background(), uuid.New(), kindMarketingEmail, false, SourceUnsubscribeLink)
	if err != nil {
		t.Fatalf("SetConsent: %v", err)
	}
	if c.OptedIn {
		t.Error("expected opted-out")
	}
}

func TestListForCustomer_MergesStoredOverDefaultsInOrder(t *testing.T) {
	repo := newFakeRepo()
	cust := uuid.New()
	// Store one row; the rest must come back as registry defaults
	repo.put(cust, kindMarketingEmail, true, SourceProfile)
	svc := NewService(repo)

	list, err := svc.ListForCustomer(context.Background(), cust)
	if err != nil {
		t.Fatalf("ListForCustomer: %v", err)
	}

	defs := AllDefinitions()
	if len(list) != len(defs) {
		t.Fatalf("got %d consents, want one per registered definition (%d)", len(list), len(defs))
	}
	// Order must match registration order (AllDefinitions order)
	for i, def := range defs {
		if list[i].Definition.Slug != def.Slug {
			t.Errorf("position %d: got slug %q, want %q (registration order not preserved)", i, list[i].Definition.Slug, def.Slug)
		}
		switch def.Slug {
		case kindMarketingEmail:
			if list[i].IsDefault || !list[i].OptedIn || list[i].Source != SourceProfile {
				t.Errorf("stored row not merged for %q: %+v", def.Slug, list[i])
			}
		default:
			if !list[i].IsDefault || list[i].OptedIn != def.DefaultOptIn {
				t.Errorf("expected default for %q: %+v", def.Slug, list[i])
			}
		}
	}
}

func TestListForCustomer_RepoErrorPropagates(t *testing.T) {
	repo := newFakeRepo()
	repo.listErr = errors.New("db down")
	svc := NewService(repo)
	_, err := svc.ListForCustomer(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestIsOptedIn_MirrorsGet(t *testing.T) {
	repo := newFakeRepo()
	cust := uuid.New()
	svc := NewService(repo)

	// No row: marketing → false, transactional → true
	if ok, err := svc.IsOptedIn(context.Background(), cust, kindMarketingEmail); err != nil || ok {
		t.Errorf("marketing no-row IsOptedIn = %v (err %v), want false", ok, err)
	}
	if ok, err := svc.IsOptedIn(context.Background(), cust, kindTransactional); err != nil || !ok {
		t.Errorf("transactional no-row IsOptedIn = %v (err %v), want true", ok, err)
	}

	// Stored opt-in flips marketing to true
	repo.put(cust, kindMarketingEmail, true, SourceProfile)
	if ok, err := svc.IsOptedIn(context.Background(), cust, kindMarketingEmail); err != nil || !ok {
		t.Errorf("after opt-in IsOptedIn = %v (err %v), want true", ok, err)
	}
}

func TestIsOptedIn_UnknownKind(t *testing.T) {
	svc := NewService(newFakeRepo())
	_, err := svc.IsOptedIn(context.Background(), uuid.New(), "nope")
	if !errors.Is(err, ErrUnknownConsent) {
		t.Fatalf("got %v, want ErrUnknownConsent", err)
	}
}

// An unsubscribe nobody hears about keeps the customer on a mailing list, so the write and the
// announcement are one operation as far as any caller is concerned
func TestOptingOutAnnouncesItself(t *testing.T) {
	svc := NewService(newFakeRepo())
	bus := events.NewBus()
	svc.SetEventBus(bus)

	var got []*ConsentChanged
	bus.Subscribe(EventChanged, func(_ context.Context, e events.Event) error {
		got = append(got, e.(*ConsentChanged))
		return nil
	})

	cid := uuid.New()
	if _, err := svc.SetConsent(context.Background(), cid, "marketing_email", false, SourceUnsubscribeLink); err != nil {
		t.Fatalf("SetConsent: %v", err)
	}
	if err := bus.WaitIdle(context.Background()); err != nil {
		t.Fatalf("drain bus: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("events published = %d, want 1 - an unsubscribe reached nobody", len(got))
	}
	ev := got[0]
	if ev.CustomerID != cid || ev.Kind != "marketing_email" {
		t.Errorf("event is for %s / %q, want %s / marketing_email", ev.CustomerID, ev.Kind, cid)
	}
	if ev.OptedIn {
		t.Error("event says opted in, for an opt-out")
	}
	if ev.Source != SourceUnsubscribeLink {
		t.Errorf("source = %q, want %q - a handler has to tell an unsubscribe from an admin edit", ev.Source, SourceUnsubscribeLink)
	}
}

// A rejected change wrote nothing, so announcing it would tell handlers a lie
func TestARefusedChangeAnnouncesNothing(t *testing.T) {
	svc := NewService(newFakeRepo())
	bus := events.NewBus()
	svc.SetEventBus(bus)

	var n int
	bus.Subscribe(EventChanged, func(context.Context, events.Event) error { n++; return nil })

	if _, err := svc.SetConsent(context.Background(), uuid.New(), "transactional_email", false, SourceProfile); !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("SetConsent err = %v, want ErrConsentRequired", err)
	}
	if _, err := svc.SetConsent(context.Background(), uuid.New(), "not_a_kind", true, SourceProfile); !errors.Is(err, ErrUnknownConsent) {
		t.Fatalf("SetConsent err = %v, want ErrUnknownConsent", err)
	}
	if err := bus.WaitIdle(context.Background()); err != nil {
		t.Fatalf("drain bus: %v", err)
	}
	if n != 0 {
		t.Errorf("events published = %d for changes that were refused, want 0", n)
	}
}

// The bus is optional, and an install without one must still record consent
func TestSetConsentWorksWithoutABus(t *testing.T) {
	svc := NewService(newFakeRepo())
	if _, err := svc.SetConsent(context.Background(), uuid.New(), "marketing_email", true, SourceProfile); err != nil {
		t.Fatalf("SetConsent: %v", err)
	}
}

func TestSetConsentRefusesAnUnknownSource(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	for _, src := range []string{"", "newsletter", "Admin"} {
		if _, err := svc.SetConsent(context.Background(), uuid.New(), kindMarketingEmail, true, src); !errors.Is(err, ErrUnknownSource) {
			t.Errorf("source %q: err = %v, want ErrUnknownSource", src, err)
		}
	}
	if len(repo.stored) != 0 || len(repo.events) != 0 {
		t.Errorf("a refused source still wrote %d rows and %d history entries", len(repo.stored), len(repo.events))
	}
}

func TestEveryChangeLeavesAHistoryEntry(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()
	cust, admin := uuid.New(), uuid.New()

	if _, err := svc.SetConsent(ctx, cust, kindMarketingEmail, true, SourceSignupForm); err != nil {
		t.Fatalf("opt in: %v", err)
	}
	if _, err := svc.SetConsent(ctx, cust, kindMarketingEmail, false, SourceUnsubscribeLink); err != nil {
		t.Fatalf("opt out: %v", err)
	}
	if _, err := svc.SetConsentByAdmin(ctx, cust, kindMarketingEmail, true, &admin); err != nil {
		t.Fatalf("admin opt in: %v", err)
	}

	hist, err := svc.History(ctx, cust, 0)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("history has %d entries, want one per change (3)", len(hist))
	}
	latest := hist[0]
	if latest.Source != SourceAdmin || !latest.OptedIn || latest.AdminID == nil || *latest.AdminID != admin {
		t.Errorf("newest entry = %+v, want the admin opt-in naming admin %s", latest, admin)
	}
	for _, e := range hist[1:] {
		if e.AdminID != nil {
			t.Errorf("%s entry names admin %s, but no admin made it", e.Source, *e.AdminID)
		}
	}
	if hist[1].Source != SourceUnsubscribeLink || hist[2].Source != SourceSignupForm {
		t.Errorf("history order = %s, %s, want newest first", hist[1].Source, hist[2].Source)
	}
}

func TestAGuestOptInIsKeptForEmailMarketingOnly(t *testing.T) {
	repo := newFakeRepo()
	ch := uuid.New()
	if err := NewService(repo).RecordEmailOptIn(context.Background(), ch, " Guest@Example.com ", SourceCheckout, "ORD-000123"); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, o := range repo.optIns {
		if o.Email != "guest@example.com" || o.Source != SourceCheckout || o.Reference != "ORD-000123" || o.ChannelID != ch {
			t.Errorf("stored %+v, want the normalized address with its proof", o)
		}
		kinds[o.Kind] = true
	}
	if !kinds[kindMarketingEmail] || kinds[kindTransactional] {
		t.Errorf("kinds %v, want email marketing and nothing transactional", kinds)
	}
	if err := NewService(repo).RecordEmailOptIn(context.Background(), ch, "a@b.c", "made_up", ""); !errors.Is(err, ErrUnknownSource) {
		t.Errorf("unknown source = %v, want ErrUnknownSource", err)
	}
}

func TestAClaimAppliesAGuestOptInUnlessTheAccountDecidedLater(t *testing.T) {
	ch, cust := uuid.New(), uuid.New()
	accountChoice := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) // what fakeRepo.put stamps
	optIn := func(at time.Time) *fakeRepo {
		repo := newFakeRepo()
		repo.optIns = []EmailOptIn{{ChannelID: ch, Email: "guest@example.com", Kind: kindMarketingEmail, Source: SourceCheckout, CreatedAt: at}}
		return repo
	}
	claim := func(repo *fakeRepo) int {
		n, err := NewService(repo).ClaimEmailOptIns(context.Background(), cust, ch, "Guest@Example.com")
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	repo := optIn(accountChoice.Add(-time.Hour))
	if n := claim(repo); n != 1 || !repo.stored[storeKey{cust, kindMarketingEmail}].OptedIn || repo.stored[storeKey{cust, kindMarketingEmail}].Source != SourceCheckout {
		t.Fatalf("first claim applied %d: %+v", n, repo.stored)
	}
	if n := claim(repo); n != 0 {
		t.Errorf("a repeat claim applied %d, want none", n)
	}

	repo = optIn(accountChoice.Add(-time.Hour))
	repo.put(cust, kindMarketingEmail, false, SourceRegistration)
	if n := claim(repo); n != 0 || repo.stored[storeKey{cust, kindMarketingEmail}].OptedIn {
		t.Errorf("an account's later no was overridden by an older tick")
	}

	repo = optIn(accountChoice.Add(time.Hour))
	repo.put(cust, kindMarketingEmail, false, SourceRegistration)
	if n := claim(repo); n != 1 || !repo.stored[storeKey{cust, kindMarketingEmail}].OptedIn {
		t.Errorf("a tick given after the account's no was not applied")
	}
}
