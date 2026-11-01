package tracking

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// fakeStore is an in-memory store: destinations by id, and the deliveries the service queued
type fakeStore struct {
	mu         sync.Mutex
	dests      map[uuid.UUID]Destination
	deliveries map[uuid.UUID]*Delivery
	keys       map[string]bool // destination+event id, the table's unique key
}

func newFakeStore(ds ...Destination) *fakeStore {
	f := &fakeStore{dests: map[uuid.UUID]Destination{}, deliveries: map[uuid.UUID]*Delivery{}, keys: map[string]bool{}}
	for _, d := range ds {
		f.dests[d.ID] = d
	}
	return f
}

func (f *fakeStore) ListEnabled(_ context.Context, ch uuid.UUID) ([]Destination, error) {
	var out []Destination
	for _, d := range f.dests {
		if d.ChannelID == ch && d.Enabled {
			out = append(out, d)
		}
	}
	return out, nil
}
func (f *fakeStore) List(ctx context.Context, ch uuid.UUID) ([]Destination, error) {
	return f.ListEnabled(ctx, ch)
}
func (f *fakeStore) Get(_ context.Context, id uuid.UUID) (Destination, error) {
	d, ok := f.dests[id]
	if !ok {
		return Destination{}, errors.New("no such destination")
	}
	return d, nil
}
func (f *fakeStore) Create(_ context.Context, d Destination) (Destination, error) {
	d.ID = uuid.New()
	f.dests[d.ID] = d
	return d, nil
}
func (f *fakeStore) Update(_ context.Context, d Destination) (Destination, error) {
	f.dests[d.ID] = d
	return d, nil
}
func (f *fakeStore) Delete(_ context.Context, id uuid.UUID) error { delete(f.dests, id); return nil }
func (f *fakeStore) Enqueue(_ context.Context, dest uuid.UUID, ev Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := dest.String() + "/" + ev.ID
	if f.keys[key] {
		return nil
	}
	f.keys[key] = true
	payload := fmt.Appendf(nil, `{"event_id":%q,"name":%q,"channel_id":%q}`, ev.ID, ev.Name, ev.ChannelID)
	id := uuid.New()
	f.deliveries[id] = &Delivery{ID: id, DestinationID: dest, EventID: ev.ID, EventName: ev.Name, Payload: payload, Status: "pending"}
	return nil
}
func (f *fakeStore) ClaimDue(_ context.Context, batch, _ int32) ([]Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Delivery
	for _, d := range f.deliveries {
		if d.Status == "pending" && !d.NextAttemptAt.After(time.Now()) && len(out) < int(batch) {
			d.Attempts++
			d.NextAttemptAt = time.Now().Add(time.Hour) // the lease
			out = append(out, *d)
		}
	}
	return out, nil
}
func (f *fakeStore) set(id uuid.UUID, fn func(*Delivery)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.deliveries[id])
	return nil
}
func (f *fakeStore) MarkSent(_ context.Context, id uuid.UUID) error {
	return f.set(id, func(d *Delivery) { d.Status = "sent" })
}
func (f *fakeStore) Reschedule(_ context.Context, id uuid.UUID, at time.Time, reason string) error {
	return f.set(id, func(d *Delivery) { d.NextAttemptAt, d.LastError = at, reason })
}
func (f *fakeStore) MarkDead(_ context.Context, id uuid.UUID, reason string) error {
	return f.set(id, func(d *Delivery) { d.Status, d.LastError = "dead", reason })
}
func (f *fakeStore) Retry(_ context.Context, id uuid.UUID) error {
	return f.set(id, func(d *Delivery) { d.Status, d.Attempts, d.NextAttemptAt = "pending", 0, time.Time{} })
}
func (f *fakeStore) ListDeliveries(context.Context, uuid.UUID, int32) ([]Delivery, error) {
	return nil, nil
}
func (f *fakeStore) Prune(context.Context, time.Time) (int64, error) { return 0, nil }

func (f *fakeStore) byDestination(id uuid.UUID) []Delivery {
	var out []Delivery
	for _, d := range f.deliveries {
		if d.DestinationID == id {
			out = append(out, *d)
		}
	}
	return out
}

type fakeSettings map[string]bool

func (f fakeSettings) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return f[path], nil
}

// registerTestKind registers a kind for one test and removes it after
func registerTestKind(t *testing.T, k Kind) {
	t.Helper()
	if k.Send == nil {
		k.Send = func(context.Context, *http.Client, Destination, Event) error { return nil }
	}
	RegisterKind(k)
	t.Cleanup(func() {
		kindsMu.Lock()
		defer kindsMu.Unlock()
		delete(kinds, k.Kind)
	})
}

func consentOf(raw string) *auth.Consent {
	c, ok := auth.ParseConsent(raw)
	if !ok {
		return nil
	}
	return &c
}

// Every combination of the visitor's choices against every purpose set a destination can need
func TestTheConsentMatrix(t *testing.T) {
	sets := map[string][]Purpose{
		"necessary":     {PurposeNecessary},
		"analytics":     {PurposeAnalytics},
		"ads":           {PurposeAds},
		"analytics+ads": {PurposeAnalytics, PurposeAds},
		"marketing":     {PurposePersonalization},
		"unknown":       {"telepathy"},
	}
	for _, consent := range []string{"", "v2:a0.d0.p0", "v2:a1.d0.p0", "v2:a0.d1.p0", "v2:a0.d0.p1", "v2:a1.d1.p0", "v2:a1.d1.p1"} {
		c := consentOf(consent)
		for name, purposes := range sets {
			want := true
			for _, p := range purposes {
				switch p {
				case PurposeNecessary:
				case PurposeAnalytics:
					want = want && c != nil && c.Analytics
				case PurposeAds:
					want = want && c != nil && c.Ads
				case PurposePersonalization:
					want = want && c != nil && c.Personalization
				default:
					want = false
				}
			}
			if got := Allowed(purposes, c); got != want {
				t.Errorf("consent %q, destination needing %s: allowed = %v, want %v", consent, name, got, want)
			}
		}
	}
}

func TestTrackQueuesOnlyWhereConsentAndTheEventListAllow(t *testing.T) {
	ch := uuid.New()
	registerTestKind(t, Kind{Kind: "t-analytics", Purposes: []Purpose{PurposeAnalytics}})
	registerTestKind(t, Kind{Kind: "t-ads", Purposes: []Purpose{PurposeAds}, DefaultEvents: []string{Purchase}})
	registerTestKind(t, Kind{Kind: "t-affiliate", Purposes: []Purpose{PurposeAds}, NecessarySetting: "general/privacy/t_necessary"})
	ga, meta, cj := uuid.New(), uuid.New(), uuid.New()
	other, off, gone := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore(
		Destination{ID: ga, ChannelID: ch, Kind: "t-analytics", Enabled: true},
		Destination{ID: meta, ChannelID: ch, Kind: "t-ads", Enabled: true},
		Destination{ID: cj, ChannelID: ch, Kind: "t-affiliate", Enabled: true},
		Destination{ID: other, ChannelID: uuid.New(), Kind: "t-analytics", Enabled: true},
		Destination{ID: off, ChannelID: ch, Kind: "t-analytics", Enabled: false},
		Destination{ID: gone, ChannelID: ch, Kind: "t-removed", Enabled: true},
	)
	svc := &Service{store: fs, settings: fakeSettings{"general/privacy/t_necessary": true}}
	ctx := context.Background()

	for _, ev := range []Event{
		{ID: "o1", Name: Purchase, ChannelID: ch, Consent: consentOf("v2:a1.d0.p0")},
		{ID: "v1", Name: ViewItem, ChannelID: ch, Consent: consentOf("v2:a1.d1.p1")},
		{ID: "n1", Name: Purchase, ChannelID: ch},                                    // no banner applied
		{ID: "o1", Name: Purchase, ChannelID: ch, Consent: consentOf("v2:a1.d0.p0")}, // a replay
	} {
		if err := svc.Track(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(dest uuid.UUID) []string {
		var out []string
		for _, d := range fs.byDestination(dest) {
			out = append(out, d.EventID)
		}
		slices.Sort(out)
		return out
	}
	for name, tc := range map[string]struct {
		dest uuid.UUID
		want []string
	}{
		"analytics gets what analytics consent allows":         {ga, []string{"o1", "v1"}},
		"ads gets only its default events, and only with ads":  {meta, nil},
		"an affiliate made necessary gets every event":         {cj, []string{"n1", "o1", "v1"}},
		"another channel's destination gets nothing":           {other, nil},
		"a disabled destination gets nothing":                  {off, nil},
		"a destination whose kind left the build gets nothing": {gone, nil},
	} {
		if got := ids(tc.dest); !slices.Equal(got, tc.want) {
			t.Errorf("%s: queued %v, want %v", name, got, tc.want)
		}
	}

	// Withdrawn mid-session: the next event goes nowhere that needed it
	if err := svc.Track(ctx, Event{ID: "v2", Name: ViewItem, ChannelID: ch, Consent: consentOf("v2:a0.d0.p0")}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(ids(ga), "v2") {
		t.Error("an event after analytics was withdrawn reached the analytics destination")
	}
	// With the setting off the affiliate needs ads consent again
	svc.settings = fakeSettings{}
	_ = svc.Track(ctx, Event{ID: "n2", Name: Purchase, ChannelID: ch})
	if slices.Contains(ids(cj), "n2") {
		t.Error("the affiliate got an unconsented event with the necessary setting off")
	}
}

func TestADestinationNeedsAKnownKindAndAName(t *testing.T) {
	registerTestKind(t, Kind{Kind: "t-known"})
	svc := &Service{store: newFakeStore()}
	if _, err := svc.Create(context.Background(), Destination{Kind: "t-nope", Name: "x"}); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("unknown kind: %v", err)
	}
	if _, err := svc.Create(context.Background(), Destination{Kind: "t-known", Name: "  "}); !errors.Is(err, ErrNameMissing) {
		t.Errorf("blank name: %v", err)
	}
	registerTestKind(t, Kind{Kind: "t-fields", Fields: []sdk.DestinationField{{Key: "id", Label: "ID", Required: true}, {Key: "token", Label: "Token", Required: true, Secret: true}}})
	ctx := context.Background()
	if _, err := svc.Create(ctx, Destination{Kind: "t-fields", Name: "x", Secret: "s"}); !errors.Is(err, ErrFieldMissing) {
		t.Errorf("missing required field: %v", err)
	}
	if _, err := svc.Create(ctx, Destination{Kind: "t-fields", Name: "x", Config: map[string]string{"id": "1"}}); !errors.Is(err, ErrFieldMissing) {
		t.Errorf("missing required secret: %v", err)
	}
	d, err := svc.Create(ctx, Destination{Kind: "t-fields", Name: "x", Config: map[string]string{"id": "1"}, Secret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	d.Secret = ""
	if _, err := svc.Update(ctx, d); err != nil {
		t.Errorf("an update that keeps the stored secret: %v", err)
	}
}

func TestTheWorkerRetriesDeadLettersAndKeepsSecretsOutOfTheLog(t *testing.T) {
	ch := uuid.New()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusNoContent)
		case "/flaky":
			w.WriteHeader(http.StatusBadGateway)
		case "/refuses":
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	registerTestKind(t, Kind{Kind: "t-http", Send: func(ctx context.Context, c *http.Client, d Destination, ev Event) error {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+d.Config["path"]+"?api_secret="+d.Secret, nil)
		res, err := c.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		switch {
		case res.StatusCode >= 500:
			return fmt.Errorf("POST %s: %d", req.URL, res.StatusCode)
		case res.StatusCode >= 400:
			return Permanent(fmt.Errorf("POST %s: %d", req.URL, res.StatusCode))
		}
		return nil
	}})
	ok, flaky, refuses := uuid.New(), uuid.New(), uuid.New()
	fs := newFakeStore(
		Destination{ID: ok, ChannelID: ch, Kind: "t-http", Enabled: true, Config: map[string]string{"path": "/ok"}, Secret: "s3cret-ok"},
		Destination{ID: flaky, ChannelID: ch, Kind: "t-http", Enabled: true, Config: map[string]string{"path": "/flaky"}, Secret: "s3cret-flaky"},
		Destination{ID: refuses, ChannelID: ch, Kind: "t-http", Enabled: true, Config: map[string]string{"path": "/refuses"}, Secret: "s3cret-refuses"},
	)
	svc := &Service{store: fs}
	ctx := context.Background()
	if err := svc.Track(ctx, Event{ID: "o1", Name: Purchase, ChannelID: ch}); err != nil {
		t.Fatal(err)
	}
	w := NewWorker(svc, srv.Client())
	w.RunOnce(ctx)

	state := func(dest uuid.UUID) Delivery { return fs.byDestination(dest)[0] }
	if s := state(ok); s.Status != "sent" {
		t.Errorf("the healthy destination's delivery is %q - a failing one held it up", s.Status)
	}
	if s := state(refuses); s.Status != "dead" || s.Attempts != 1 {
		t.Errorf("a 4xx refusal: %+v, want dead after one attempt", s)
	}
	f := state(flaky)
	if f.Status != "pending" || f.NextAttemptAt.Before(time.Now().Add(5*time.Second)) {
		t.Errorf("a 5xx: %+v, want pending and rescheduled about 10s out", f)
	}
	for _, d := range []Delivery{state(refuses), f} {
		if strings.Contains(d.LastError, "s3cret") || !strings.Contains(d.LastError, "[redacted]") {
			t.Errorf("the log kept a secret: %q", d.LastError)
		}
	}

	// Run the ladder out: the flaky row is retried until it is dead, never more
	for range maxAttempts {
		_ = fs.set(f.ID, func(d *Delivery) { d.NextAttemptAt = time.Time{} })
		w.RunOnce(ctx)
	}
	if s := state(flaky); s.Status != "dead" || s.Attempts != maxAttempts || hits["/flaky"] != int(maxAttempts) {
		t.Errorf("after the ladder: %+v with %d calls, want dead after %d", s, hits["/flaky"], maxAttempts)
	}
	if err := svc.Redeliver(ctx, f.ID); err != nil || state(flaky).Status != "pending" {
		t.Error("a dead letter must go back in the queue on redelivery")
	}
}

func TestDecimalFollowsTheCurrency(t *testing.T) {
	for _, tc := range []struct {
		minor    int64
		currency string
		want     float64
	}{{1999, "EUR", 19.99}, {1999, "JPY", 1999}, {1999, "KWD", 1.999}} {
		if got := Decimal(tc.minor, tc.currency); got != tc.want {
			t.Errorf("Decimal(%d, %s) = %v, want %v", tc.minor, tc.currency, got, tc.want)
		}
	}
}

func TestASendTestGoesStraightToThePlatform(t *testing.T) {
	var got Event
	registerTestKind(t, Kind{Kind: "t-direct", Send: func(_ context.Context, _ *http.Client, d Destination, ev Event) error {
		got = ev
		return Permanent(errors.New("400 " + d.Name))
	}})
	id := uuid.New()
	svc := &Service{store: newFakeStore(Destination{ID: id, ChannelID: uuid.New(), Kind: "t-direct", Name: "acct", Enabled: true})}
	ev, err := svc.SendTest(context.Background(), http.DefaultClient, id)
	if err == nil || !strings.Contains(err.Error(), "400 acct") || got.ID != ev.ID || got.Name != Purchase || !strings.HasPrefix(ev.ID, "test-") {
		t.Errorf("err %v, sent %+v", err, got)
	}
}
