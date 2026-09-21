package locale

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	rows       map[string]Locale
	created    []CreateLocaleInput
	updated    []UpdateLocaleInput
	deleted    []string
	references int64
	createErr  error
	updateErr  error
	countErr   error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string]Locale{}} }

func (r *fakeRepo) Create(_ context.Context, in CreateLocaleInput) (Locale, error) {
	if r.createErr != nil {
		return Locale{}, r.createErr
	}
	r.created = append(r.created, in)
	l := Locale{
		Code: in.Code, Name: in.Name, NativeName: in.NativeName, IsActive: in.IsActive,
		DecimalSeparator: in.DecimalSeparator, ThousandsSeparator: in.ThousandsSeparator,
		FirstDayOfWeek: in.FirstDayOfWeek, DefaultCountryCode: in.DefaultCountryCode,
		HreflangValue: in.HreflangValue,
	}
	r.rows[in.Code] = l
	return l, nil
}

func (r *fakeRepo) GetByCode(_ context.Context, code string) (Locale, error) {
	l, ok := r.rows[code]
	if !ok {
		return Locale{}, ErrLocaleNotFound
	}
	return l, nil
}

func (r *fakeRepo) List(_ context.Context, activeOnly bool) ([]Locale, error) {
	out := make([]Locale, 0, len(r.rows))
	for _, l := range r.rows {
		if activeOnly && !l.IsActive {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

func (r *fakeRepo) Update(_ context.Context, in UpdateLocaleInput) (Locale, error) {
	if r.updateErr != nil {
		return Locale{}, r.updateErr
	}
	r.updated = append(r.updated, in)
	l := r.rows[in.Code]
	l.Code = in.Code
	if in.Name != nil {
		l.Name = *in.Name
	}
	if in.IsActive != nil {
		l.IsActive = *in.IsActive
	}
	if in.DefaultCountryCode != nil {
		l.DefaultCountryCode = *in.DefaultCountryCode
	}
	if in.HreflangValue != nil {
		l.HreflangValue = *in.HreflangValue
	}
	r.rows[in.Code] = l
	return l, nil
}

func (r *fakeRepo) Delete(_ context.Context, code string) error {
	r.deleted = append(r.deleted, code)
	delete(r.rows, code)
	return nil
}

func (r *fakeRepo) CountReferences(context.Context, string) (int64, error) {
	if r.countErr != nil {
		return 0, r.countErr
	}
	return r.references, nil
}

type stubCountries struct {
	known map[string]bool
	calls []string
}

func (s *stubCountries) EnsureExists(_ context.Context, code string) error {
	s.calls = append(s.calls, code)
	if !s.known[code] {
		return errors.New("country not found: " + code)
	}
	return nil
}

func newTestService() (*Service, *fakeRepo) {
	repo := newFakeRepo()
	return NewService(repo), repo
}

// A half-filled row would blow up storefront number and date rendering, so the service fills in the
// separators and native name rather than letting a NULL reach a template
func TestCreateFillsInTheRenderingDefaults(t *testing.T) {
	svc, repo := newTestService()

	got, err := svc.Create(context.Background(), CreateLocaleInput{Code: "de", Name: "German"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.NativeName != "German" {
		t.Errorf("NativeName = %q, want it defaulted to the name", got.NativeName)
	}
	if got.DecimalSeparator != "." || got.ThousandsSeparator != "," {
		t.Errorf("separators = %q / %q, want the . and , defaults", got.DecimalSeparator, got.ThousandsSeparator)
	}
	if got.FirstDayOfWeek != 1 {
		t.Errorf("FirstDayOfWeek = %d, want Monday when the caller expressed no opinion", got.FirstDayOfWeek)
	}
	if len(repo.created) != 1 {
		t.Fatalf("%d rows created, want 1", len(repo.created))
	}
	// an explicit value is never overwritten
	if _, err := svc.Create(context.Background(), CreateLocaleInput{
		Code: "fr", Name: "French", NativeName: "Francais", DecimalSeparator: ",", ThousandsSeparator: " ",
	}); err != nil {
		t.Fatalf("Create fr: %v", err)
	}
	fr := repo.created[1]
	if fr.NativeName != "Francais" || fr.DecimalSeparator != "," || fr.ThousandsSeparator != " " {
		t.Errorf("explicit values were overwritten: %+v", fr)
	}
}

// The code is the primary key every content row joins on, so a stray case or space would split one
// locale into two
func TestCreateNormalizesTheCode(t *testing.T) {
	svc, repo := newTestService()
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: "  DE-AT  ", Name: "Austrian German"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := repo.created[0].Code; got != "de-at" {
		t.Errorf("stored code = %q, want %q", got, "de-at")
	}
	if _, err := svc.GetByCode(context.Background(), " DE-AT "); err != nil {
		t.Errorf("GetByCode with the caller's casing = %v, want the row found", err)
	}
}

func TestCreateRejectsBadInput(t *testing.T) {
	svc, repo := newTestService()
	for name, in := range map[string]CreateLocaleInput{
		"empty code":           {Code: "", Name: "X"},
		"underscore code":      {Code: "de_AT", Name: "X"},
		"not a code":           {Code: "!!", Name: "X"},
		"one letter":           {Code: "d", Name: "X"},
		"blank name":           {Code: "de", Name: "   "},
		"bad country":          {Code: "de", Name: "German", DefaultCountryCode: "DEU"},
		"bad hreflang":         {Code: "de", Name: "German", HreflangValue: "de_AT"},
		"day of week too big":  {Code: "de", Name: "German", FirstDayOfWeek: 7, DefaultCountryCode: "DE"},
		"day of week negative": {Code: "de", Name: "German", FirstDayOfWeek: -1, DefaultCountryCode: "DE"},
	} {
		if _, err := svc.Create(context.Background(), in); err == nil {
			t.Errorf("%s was accepted, want a validation error", name)
		}
	}
	if len(repo.created) != 0 {
		t.Errorf("%d rows reached the repository despite validation failing", len(repo.created))
	}
}

// Presentation fields are normalized before storing so hreflang renders lowercase in the page head and the country code matches the countries table's casing Create and Update must normalize in the same order - trimming on one side only means a padded country code the edit screen accepts is refused on the create screen
func TestCreateNormalizesCountryAndHreflang(t *testing.T) {
	svc, repo := newTestService()
	if _, err := svc.Create(context.Background(), CreateLocaleInput{
		Code: "de-at", Name: "Austrian German", DefaultCountryCode: " at ", HreflangValue: " DE-AT ",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got := repo.created[0]
	if got.DefaultCountryCode != "AT" {
		t.Errorf("country = %q, want it uppercased and trimmed", got.DefaultCountryCode)
	}
	if got.HreflangValue != "de-at" {
		t.Errorf("hreflang = %q, want it lowercased and trimmed", got.HreflangValue)
	}
}

// Without the pre-check the admin sees a raw foreign-key violation instead of a readable message
func TestCreateChecksTheCountryExistsWhenAValidatorIsWired(t *testing.T) {
	svc, repo := newTestService()
	countries := &stubCountries{known: map[string]bool{"AT": true}}
	svc.SetCountryValidator(countries)

	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: "de-at", Name: "X", DefaultCountryCode: "at"}); err != nil {
		t.Errorf("a known country = %v, want nil", err)
	}
	if len(countries.calls) != 1 || countries.calls[0] != "AT" {
		t.Errorf("validator saw %v, want the normalized [AT]", countries.calls)
	}
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: "de-ch", Name: "X", DefaultCountryCode: "CH"}); err == nil {
		t.Error("an unknown country was accepted")
	}
	if len(repo.created) != 1 {
		t.Errorf("%d rows created, want only the one with a known country", len(repo.created))
	}

	// no country supplied means the validator is never consulted
	countries.calls = nil
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: "es", Name: "Spanish"}); err != nil {
		t.Fatalf("Create es: %v", err)
	}
	if len(countries.calls) != 0 {
		t.Errorf("validator was called with %v for a locale carrying no country", countries.calls)
	}
}

func TestUpdateValidatesOnlyTheSuppliedFields(t *testing.T) {
	svc, repo := newTestService()
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: "de", Name: "German"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	name := "Deutsch"
	if _, err := svc.Update(context.Background(), UpdateLocaleInput{Code: "DE", Name: &name}); err != nil {
		t.Errorf("a name-only update = %v, want nil", err)
	}
	if got := repo.updated[0].Code; got != "de" {
		t.Errorf("update code = %q, want it normalized", got)
	}

	blank := "  "
	badDay := int16(9)
	badCountry := "DEU"
	badHreflang := "de_AT"
	for name, in := range map[string]UpdateLocaleInput{
		"blank name":   {Code: "de", Name: &blank},
		"bad day":      {Code: "de", FirstDayOfWeek: &badDay},
		"bad country":  {Code: "de", DefaultCountryCode: &badCountry},
		"bad hreflang": {Code: "de", HreflangValue: &badHreflang},
	} {
		if _, err := svc.Update(context.Background(), in); err == nil {
			t.Errorf("%s was accepted, want a validation error", name)
		}
	}
}

func TestUpdateNormalizesCountryAndHreflang(t *testing.T) {
	svc, repo := newTestService()
	country, hreflang := " at ", " DE-AT "
	if _, err := svc.Update(context.Background(), UpdateLocaleInput{
		Code: "de-at", DefaultCountryCode: &country, HreflangValue: &hreflang,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := repo.updated[0]
	if *got.DefaultCountryCode != "AT" {
		t.Errorf("country = %q, want AT", *got.DefaultCountryCode)
	}
	if *got.HreflangValue != "de-at" {
		t.Errorf("hreflang = %q, want de-at", *got.HreflangValue)
	}
}

// The hook reindexes the search docs for a locale, so it has to fire on a real transition only -
// re-saving an already-active locale would otherwise kick off a full reindex every time
func TestUpdateFiresTheStatusHookOnlyOnARealFlip(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: "de", Name: "German", IsActive: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	type flip struct {
		code   string
		active bool
	}
	var flips []flip
	svc.SetStatusChangeHook(func(_ context.Context, code string, isActive bool) {
		flips = append(flips, flip{code, isActive})
	})

	stillActive := true
	if _, err := svc.Update(context.Background(), UpdateLocaleInput{Code: "de", IsActive: &stillActive}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(flips) != 0 {
		t.Errorf("hook fired %v for a no-op status write, want nothing", flips)
	}

	off := false
	if _, err := svc.Update(context.Background(), UpdateLocaleInput{Code: "de", IsActive: &off}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(flips) != 1 || flips[0].code != "de" || flips[0].active {
		t.Fatalf("hook saw %v, want one deactivation of de", flips)
	}

	on := true
	if _, err := svc.Update(context.Background(), UpdateLocaleInput{Code: "de", IsActive: &on}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(flips) != 2 || !flips[1].active {
		t.Errorf("hook saw %v, want a reactivation as the second entry", flips)
	}

	// an update that doesn't carry IsActive can't be a flip
	name := "Deutsch"
	if _, err := svc.Update(context.Background(), UpdateLocaleInput{Code: "de", Name: &name}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(flips) != 2 {
		t.Errorf("hook fired %v on an update with no status field", flips)
	}
}

// A failed write must not tell the indexer a flip happened
func TestUpdateSkipsTheHookWhenTheWriteFails(t *testing.T) {
	svc, repo := newTestService()
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: "de", Name: "German", IsActive: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var fired int
	svc.SetStatusChangeHook(func(context.Context, string, bool) { fired++ })
	repo.updateErr = errors.New("db down")

	off := false
	if _, err := svc.Update(context.Background(), UpdateLocaleInput{Code: "de", IsActive: &off}); err == nil {
		t.Fatal("Update = nil, want the repository error surfaced")
	}
	if fired != 0 {
		t.Errorf("hook fired %d times after a failed write, want 0", fired)
	}
}

// Deleting a referenced locale would leave orphaned content, so the count is the guard and the admin
// gets told how many rows to resolve
func TestDeleteRefusesWhileReferenced(t *testing.T) {
	svc, repo := newTestService()
	repo.references = 47

	err := svc.Delete(context.Background(), "DE")
	if !errors.Is(err, ErrLocaleInUse) {
		t.Fatalf("Delete with references = %v, want ErrLocaleInUse", err)
	}
	if len(repo.deleted) != 0 {
		t.Error("the row was deleted despite the reference guard")
	}

	repo.references = 0
	if err := svc.Delete(context.Background(), "DE"); err != nil {
		t.Fatalf("Delete with no references: %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != "de" {
		t.Errorf("deleted %v, want the normalized [de]", repo.deleted)
	}
}

func TestDeletePropagatesACountFailure(t *testing.T) {
	svc, repo := newTestService()
	repo.countErr = errors.New("db down")
	if err := svc.Delete(context.Background(), "de"); !errors.Is(err, repo.countErr) {
		t.Errorf("Delete with a failing count = %v, want %v", err, repo.countErr)
	}
	if len(repo.deleted) != 0 {
		t.Error("the row was deleted even though the reference count could not be read")
	}
}

func TestEnsureExists(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Create(context.Background(), CreateLocaleInput{Code: "de", Name: "German"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.EnsureExists(context.Background(), " DE "); err != nil {
		t.Errorf("EnsureExists on a known locale = %v, want nil", err)
	}
	if err := svc.EnsureExists(context.Background(), "zz"); !errors.Is(err, ErrLocaleNotFound) {
		t.Errorf("EnsureExists on an unknown locale = %v, want ErrLocaleNotFound", err)
	}
}

type stubDefault struct {
	code string
	err  error
}

func (s stubDefault) DefaultLocale(context.Context) (string, error) { return s.code, s.err }

// Every localized read resolves through this chain, so a wrong precedence silently serves the wrong
// language to a request that explicitly asked for one
func TestActiveResolutionOrder(t *testing.T) {
	ctx := context.Background()
	withHeader := WithLocale(ctx, "de-AT")

	if got := Active(withHeader, stubDefault{code: "en"}); got != "de-at" {
		t.Errorf("Active with a request locale = %q, want the request's normalized tag", got)
	}
	if got := Active(ctx, stubDefault{code: " en "}); got != "en" {
		t.Errorf("Active with no request locale = %q, want the trimmed system default", got)
	}
	if got := Active(ctx, nil); got != "" {
		t.Errorf("Active with nothing = %q, want the empty no-preference value", got)
	}
	if got := Active(ctx, stubDefault{err: errors.New("db down")}); got != "" {
		t.Errorf("Active with a failing provider = %q, want it to fall through to no preference", got)
	}
	// a request locale wins even when the provider is broken
	if got := Active(withHeader, stubDefault{err: errors.New("db down")}); got != "de-at" {
		t.Errorf("Active = %q, want the request locale regardless of the provider", got)
	}
}

// Both write paths take the same input, so an admin form's padding can't be accepted on one screen
// and refused on the other
func TestCreateAndUpdateAgreeOnPaddedInput(t *testing.T) {
	for _, val := range []string{" at ", "at", "AT ", "AT"} {
		svc, _ := newTestService()
		_, cerr := svc.Create(context.Background(), CreateLocaleInput{Code: "de-at", Name: "X", DefaultCountryCode: val})
		v := val
		_, uerr := svc.Update(context.Background(), UpdateLocaleInput{Code: "de-at", DefaultCountryCode: &v})
		if (cerr != nil) != (uerr != nil) {
			t.Errorf("country %q: Create err=%v but Update err=%v", val, cerr, uerr)
		}
	}
	for _, val := range []string{" de-AT ", "de-AT", "DE-AT"} {
		svc, _ := newTestService()
		_, cerr := svc.Create(context.Background(), CreateLocaleInput{Code: "de-at", Name: "X", HreflangValue: val})
		v := val
		_, uerr := svc.Update(context.Background(), UpdateLocaleInput{Code: "de-at", HreflangValue: &v})
		if (cerr != nil) != (uerr != nil) {
			t.Errorf("hreflang %q: Create err=%v but Update err=%v", val, cerr, uerr)
		}
	}
}
