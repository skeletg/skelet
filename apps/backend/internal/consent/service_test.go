package consent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRepo is a hand-written in-memory Repository - Get returns a nil *StoredConsent for any (customer, kind) not explicitly stored,
// mirroring the real postgres repo's sql.ErrNoRows → nil contract that drives the service's default-fallback logic
type fakeRepo struct {
	stored    map[storeKey]StoredConsent
	getErr    error
	upsertErr error
	listErr   error
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

func (r *fakeRepo) Upsert(_ context.Context, customerID uuid.UUID, kind string, optedIn bool, source string) (StoredConsent, error) {
	if r.upsertErr != nil {
		return StoredConsent{}, r.upsertErr
	}
	r.put(customerID, kind, optedIn, source)
	return r.stored[storeKey{customerID, kind}], nil
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

func (r *fakeRepo) ListByCustomerIDs(_ context.Context, customerIDs []uuid.UUID, kind string) ([]StoredConsent, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	want := make(map[uuid.UUID]struct{}, len(customerIDs))
	for _, id := range customerIDs {
		want[id] = struct{}{}
	}
	var out []StoredConsent
	for k, v := range r.stored {
		if k.kind != kind {
			continue
		}
		if _, ok := want[k.customer]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *fakeRepo) DeleteByCustomer(_ context.Context, customerID uuid.UUID) error {
	for k := range r.stored {
		if k.customer == customerID {
			delete(r.stored, k)
		}
	}
	return nil
}

// The built-in kinds registered by defaults.go init() - Pinning these keeps
// the tests honest about the GDPR defaults the service relies on
const (
	kindMarketingEmail = "marketing_email"     // Required=false, DefaultOptIn=false
	kindTransactional  = "transactional_email" // Required=true,  DefaultOptIn=true
)

func TestOptedInByCustomerIDs_MatchesIsOptedIn(t *testing.T) {
	repo := newFakeRepo()
	optedIn := uuid.New()
	optedOut := uuid.New()
	noRow := uuid.New() // must fall back to DefaultOptIn (false for marketing)
	repo.put(optedIn, kindMarketingEmail, true, "test")
	repo.put(optedOut, kindMarketingEmail, false, "test")
	svc := NewService(repo)

	got, err := svc.OptedInByCustomerIDs(context.Background(), []uuid.UUID{optedIn, optedOut, noRow}, kindMarketingEmail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Batch result must match IsOptedIn per customer - especially the no-row
	// case, which must NOT be sent to (default opt-out)
	for _, cust := range []uuid.UUID{optedIn, optedOut, noRow} {
		want, _ := svc.IsOptedIn(context.Background(), cust, kindMarketingEmail)
		if got[cust] != want {
			t.Errorf("customer %s: batch=%v, IsOptedIn=%v", cust, got[cust], want)
		}
	}
	if got[noRow] {
		t.Error("no-row customer must default to opted-out for marketing")
	}
}

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
