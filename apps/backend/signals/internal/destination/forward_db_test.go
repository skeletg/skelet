package destination

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type openGuard struct{}

func (openGuard) ValidateTargetURL(string, bool) error { return nil }
func (openGuard) SafeTransport(bool) *http.Transport   { return &http.Transport{} }

type fakeSpine struct {
	mu     sync.Mutex
	events map[uuid.UUID][]analytics.ForwardEvent
}

func (f *fakeSpine) add(channelID uuid.UUID, n int, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.events == nil {
		f.events = map[uuid.UUID][]analytics.ForwardEvent{}
	}
	at := time.Now().Add(time.Second).Truncate(time.Microsecond) // the cursor round-trips through Postgres, which keeps microseconds
	if evs := f.events[channelID]; len(evs) > 0 {
		at = evs[len(evs)-1].CreatedAt
	}
	for range n {
		at = at.Add(time.Microsecond)
		f.events[channelID] = append(f.events[channelID], analytics.ForwardEvent{
			ID: uuid.New(), Name: name, Source: "web", AnonID: "anon-1", OccurredAt: at, CreatedAt: at, Properties: json.RawMessage(`{}`),
		})
	}
}

func (f *fakeSpine) EventsForForwarding(_ context.Context, channelID uuid.UUID, afterAt time.Time, afterID uuid.UUID, limit int) ([]analytics.ForwardEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []analytics.ForwardEvent
	for _, e := range f.events[channelID] {
		if e.CreatedAt.After(afterAt) || (e.CreatedAt.Equal(afterAt) && strings.Compare(e.ID.String(), afterID.String()) > 0) {
			out = append(out, e)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

type received struct {
	query  string
	header http.Header
	body   []byte
}

type receiver struct {
	*httptest.Server
	mu     sync.Mutex
	got    []received
	status int
}

func newReceiver(t *testing.T) *receiver {
	rc := &receiver{status: http.StatusOK}
	rc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rc.mu.Lock()
		rc.got = append(rc.got, received{query: r.URL.RawQuery, header: r.Header.Clone(), body: b})
		status := rc.status
		rc.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(rc.Close)
	return rc
}

func (rc *receiver) requests() []received {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return slices.Clone(rc.got)
}

func (rc *receiver) setStatus(s int) {
	rc.mu.Lock()
	rc.status = s
	rc.mu.Unlock()
}

func forwardedNames(t *testing.T, reqs []received) []string {
	t.Helper()
	var names []string
	for _, r := range reqs {
		var body struct {
			Events []struct {
				Name string `json:"name"`
			} `json:"events"`
		}
		if err := json.Unmarshal(r.body, &body); err != nil {
			t.Fatalf("decode %s: %v", r.body, err)
		}
		for _, e := range body.Events {
			names = append(names, e.Name)
		}
	}
	return names
}

type harness struct {
	svc   *Service
	conn  *sql.DB
	spine *fakeSpine
}

func newHarness(t *testing.T, sealer Sealer) harness {
	t.Helper()
	conn := testdb.RequireSignals(t)
	// The sweep reads every active destination, so each test starts from none
	if _, err := conn.Exec(`DELETE FROM event_destinations`); err != nil {
		t.Fatal(err)
	}
	spine := &fakeSpine{}
	return harness{svc: NewService(signalsdb.New(conn), spine, openGuard{}, sealer, true), conn: conn, spine: spine}
}

func (h harness) create(t *testing.T, in UpsertInput) Destination {
	t.Helper()
	if in.ChannelID == uuid.Nil {
		in.ChannelID = uuid.New()
	}
	if in.Name == "" {
		in.Name = "dest"
	}
	if in.Kind == "" {
		in.Kind = KindWebhook
	}
	in.IsActive = true
	d, err := h.svc.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (h harness) forward(t *testing.T) {
	t.Helper()
	if err := h.svc.ForwardAll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (h harness) lastError(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := h.conn.QueryRow(`SELECT last_error FROM event_destinations WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func testCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	c, err := secrets.NewCipher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestForwardingSignsTheBatchAndAdvancesTheCursor(t *testing.T) {
	h := newHarness(t, testCipher(t))
	rc := newReceiver(t)
	d := h.create(t, UpsertInput{TargetURL: rc.URL, Secret: "whsec"})
	h.spine.add(d.ChannelID, 3, "page_view")

	h.forward(t)
	h.forward(t)
	reqs := rc.requests()
	if len(reqs) != 1 {
		t.Fatalf("want one delivery across two sweeps, got %d", len(reqs))
	}
	if n := len(forwardedNames(t, reqs)); n != 3 {
		t.Errorf("forwarded %d events, want 3", n)
	}
	mac := hmac.New(sha256.New, []byte("whsec"))
	mac.Write([]byte(reqs[0].header.Get("X-Destination-Timestamp") + "."))
	mac.Write(reqs[0].body)
	if got := reqs[0].header.Get("X-Destination-Signature"); got != hex.EncodeToString(mac.Sum(nil)) {
		t.Errorf("signature %q does not verify with the plaintext secret", got)
	}
}

func TestAFailedDeliveryKeepsTheCursor(t *testing.T) {
	h := newHarness(t, nil)
	rc := newReceiver(t)
	d := h.create(t, UpsertInput{TargetURL: rc.URL})
	h.spine.add(d.ChannelID, 2, "page_view")

	rc.setStatus(http.StatusInternalServerError)
	h.forward(t)
	if e := h.lastError(t, d.ID); !strings.Contains(e, "HTTP 500") {
		t.Errorf("last_error = %q, want the HTTP 500", e)
	}
	rc.setStatus(http.StatusOK)
	h.forward(t)
	reqs := rc.requests()
	if len(reqs) != 2 || len(forwardedNames(t, reqs[1:])) != 2 {
		t.Fatalf("the retry should resend both events, got %d requests", len(reqs))
	}
	if e := h.lastError(t, d.ID); e != "" {
		t.Errorf("a successful sweep should clear last_error, got %q", e)
	}
}

func TestFiltersDropUnmatchedEventsButStillAdvance(t *testing.T) {
	h := newHarness(t, nil)
	rc := newReceiver(t)
	d := h.create(t, UpsertInput{TargetURL: rc.URL, EventNames: []string{"Order_Placed"}})
	h.spine.add(d.ChannelID, 2, "page_view")
	h.spine.add(d.ChannelID, 1, "order_placed")

	h.forward(t)
	h.forward(t)
	if names := forwardedNames(t, rc.requests()); !slices.Equal(names, []string{"order_placed"}) {
		t.Errorf("forwarded %v, want only the matching event, once", names)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	h := newHarness(t, nil)
	elsewhere := newReceiver(t)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)
	d := h.create(t, UpsertInput{TargetURL: redirector.URL, Secret: "whsec"})
	h.spine.add(d.ChannelID, 1, "page_view")

	h.forward(t)
	if n := len(elsewhere.requests()); n != 0 {
		t.Fatalf("the signed batch followed the redirect %d time(s)", n)
	}
	if e := h.lastError(t, d.ID); !strings.Contains(e, "redirects are not followed") {
		t.Errorf("last_error = %q, want it to name the redirect", e)
	}
}

func TestABackloggedDestinationYieldsToTheRest(t *testing.T) {
	h := newHarness(t, nil)
	busy, quiet := newReceiver(t), newReceiver(t)
	b := h.create(t, UpsertInput{TargetURL: busy.URL})
	q := h.create(t, UpsertInput{TargetURL: quiet.URL})
	h.spine.add(b.ChannelID, batchSize*(maxBatchesPerRun+2), "page_view")
	h.spine.add(q.ChannelID, 1, "page_view")

	h.forward(t)
	if n := len(busy.requests()); n != maxBatchesPerRun {
		t.Errorf("backlogged destination sent %d batches in one sweep, want %d", n, maxBatchesPerRun)
	}
	if n := len(quiet.requests()); n != 1 {
		t.Errorf("the destination behind the backlog got %d deliveries, want 1", n)
	}
	h.forward(t)
	if n := len(busy.requests()); n != maxBatchesPerRun+2 {
		t.Errorf("the next sweep should pick up where the cap stopped, got %d batches total", n)
	}
}

func TestTheGA4CredentialRidesTheQueryAndNeverTheError(t *testing.T) {
	h := newHarness(t, testCipher(t))
	rc := newReceiver(t)
	d := h.create(t, UpsertInput{Kind: KindGA4, TargetURL: rc.URL + "/mp/collect?measurement_id=G-1", Secret: "s3cr3t"})
	h.spine.add(d.ChannelID, 1, "page_view")
	h.forward(t)
	reqs := rc.requests()
	if len(reqs) != 1 || !strings.Contains(reqs[0].query, "api_secret=s3cr3t") || !strings.Contains(reqs[0].query, "measurement_id=G-1") {
		t.Fatalf("GA4 request query = %v, want measurement_id and api_secret", reqs)
	}

	dead := h.create(t, UpsertInput{Kind: KindGA4, TargetURL: "http://127.0.0.1:1/mp/collect?measurement_id=G-2", Secret: "s3cr3t"})
	h.spine.add(dead.ChannelID, 1, "page_view")
	h.forward(t)
	if e := h.lastError(t, dead.ID); e == "" || strings.Contains(e, "s3cr3t") {
		t.Errorf("last_error = %q, want the failure without the credential", e)
	}
	for _, x := range []Destination{d, dead} {
		if strings.Contains(x.TargetURL, "s3cr3t") {
			t.Errorf("the credential leaked into the stored target URL: %s", x.TargetURL)
		}
	}
}

func TestSecretsAreSealedAtRest(t *testing.T) {
	h := newHarness(t, testCipher(t))
	d := h.create(t, UpsertInput{TargetURL: "https://hooks.example.com/x", Secret: "whsec-plain"})
	var stored string
	if err := h.conn.QueryRow(`SELECT secret FROM event_destinations WHERE id = $1`, d.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !secrets.IsEncrypted(stored) || strings.Contains(stored, "whsec-plain") {
		t.Fatalf("stored secret %q is not sealed", stored)
	}

	// Without the key a sealed secret refuses rather than signing with ciphertext
	keyless := NewService(signalsdb.New(h.conn), h.spine, openGuard{}, nil, true)
	h.spine.add(d.ChannelID, 1, "page_view")
	if err := keyless.ForwardAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e := h.lastError(t, d.ID); !strings.Contains(e, "SIGNALS_ENCRYPTION_KEY") {
		t.Errorf("last_error = %q, want the missing key named", e)
	}
}

func TestAudienceSyncIsSignedAndWebhookOnly(t *testing.T) {
	h := newHarness(t, testCipher(t))
	segID := uuid.New()
	if _, err := h.conn.Exec(`INSERT INTO customer_segments (id, slug, name) VALUES ($1, $2, 'VIP')`, segID, "vip-"+segID.String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = h.conn.Exec(`DELETE FROM customer_segments WHERE id = $1`, segID) })
	rc := newReceiver(t)
	hook := h.create(t, UpsertInput{TargetURL: rc.URL, Secret: "whsec"})
	ga4 := h.create(t, UpsertInput{Kind: KindGA4, TargetURL: rc.URL, Secret: "s3cr3t"})

	if _, err := h.svc.SyncAudience(context.Background(), segID, ga4.ID, []string{"h1"}); !errors.Is(err, ErrAudienceNeedsWebhook) {
		t.Fatalf("audience sync to GA4: err = %v, want ErrAudienceNeedsWebhook", err)
	}
	n, err := h.svc.SyncAudience(context.Background(), segID, hook.ID, []string{"h1", "h2"})
	if err != nil || n != 2 {
		t.Fatalf("sync = %d, %v", n, err)
	}
	reqs := rc.requests()
	if len(reqs) != 1 || reqs[0].header.Get("X-Destination-Signature") == "" {
		t.Fatalf("audience sync went out unsigned: %+v", reqs)
	}
	if _, err := h.svc.SyncAudience(context.Background(), segID, uuid.New(), nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown destination: err = %v, want ErrNotFound", err)
	}
}
