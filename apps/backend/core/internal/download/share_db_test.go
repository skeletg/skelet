package download

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

func (r rig) fileAndCustomer(t *testing.T) (assetID, customerID uuid.UUID) {
	t.Helper()
	key := "contracts/" + uuid.NewString() + "-price-list.pdf"
	if err := r.conn.QueryRow(`INSERT INTO media_assets (id, directory_prefix, filename, object_key, mime_type, size_bytes, status, visibility)
		VALUES ($2, 'contracts', 'price-list.pdf', $1, 'application/pdf', 10, 'ready', 'private') RETURNING id`, key, uuid.New()).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM media_assets WHERE id = $1`, assetID) })
	return assetID, uuid.New()
}

func TestAFileSharedWithACustomerJoinsTheirDownloads(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	assetID, customerID := r.fileAndCustomer(t)

	first, err := r.svc.ShareAsset(ctx, ShareInput{AssetID: assetID, CustomerID: customerID, Email: "ana@example.test", Title: "Price list 2027"})
	if err != nil {
		t.Fatal(err)
	}
	if first.AssetID == nil || *first.AssetID != assetID || first.OrderID != nil || first.Token == "" || first.MaxDownloads != nil {
		t.Fatalf("the share came back as %+v", first)
	}
	mine, err := r.svc.ListForCustomer(ctx, customerID)
	if err != nil || len(mine) != 1 || mine[0].Title != "Price list 2027" {
		t.Fatalf("the customer's downloads: %v %v, want the shared file", mine, err)
	}

	if _, err := r.svc.Consume(ctx, first.Token); err != nil {
		t.Fatal(err)
	}
	limit, until := int32(3), time.Now().Add(48*time.Hour).UTC().Truncate(time.Second)
	again, err := r.svc.ShareAsset(ctx, ShareInput{AssetID: assetID, CustomerID: customerID, Email: "ana@example.test", Title: "Price list 2027", MaxDownloads: &limit, ExpiresAt: &until})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || again.Token != first.Token || again.MaxDownloads == nil || *again.MaxDownloads != 3 || again.ExpiresAt == nil || !again.ExpiresAt.Equal(until) {
		t.Fatalf("sharing again gave %+v, want the same grant with the new limits", again)
	}
	if again.DownloadsUsed != 0 {
		t.Errorf("sharing again kept %d downloads already used, want it started over", again.DownloadsUsed)
	}
	shares, err := r.svc.SharesOf(ctx, assetID)
	if err != nil || len(shares) != 1 {
		t.Fatalf("shares of the file: %v %v, want one", shares, err)
	}

	if err := r.svc.Unshare(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if mine, _ := r.svc.ListForCustomer(ctx, customerID); len(mine) != 0 {
		t.Errorf("after revoking, the customer still has %d downloads", len(mine))
	}
	if _, err := r.svc.Get(ctx, first.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("the revoked link still resolves: %v", err)
	}
}

func TestAShareIsHeldToSaneLimitsAndOnlySharesCanBeRevokedAsShares(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	assetID, customerID := r.fileAndCustomer(t)
	zero, past := int32(0), time.Now().Add(-time.Hour)
	for name, in := range map[string]ShareInput{
		"a limit of zero":       {AssetID: assetID, CustomerID: customerID, MaxDownloads: &zero},
		"an expiry in the past": {AssetID: assetID, CustomerID: customerID, ExpiresAt: &past},
	} {
		if _, err := r.svc.ShareAsset(ctx, in); !errors.Is(err, ErrInvalidShare) {
			t.Errorf("%s: %v, want ErrInvalidShare", name, err)
		}
	}

	r.orders.set("pending", order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	if err := r.svc.Unshare(ctx, r.grants(t)[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoking a paid download as if it were a share: %v, want ErrNotFound", err)
	}
	if len(r.grants(t)) != 1 {
		t.Error("the paid download was deleted")
	}
}

func TestASharedFileIsStreamedUnderItsOwnNameAndGoesWithTheFile(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	assetID, customerID := r.fileAndCustomer(t)
	limit := int32(1)
	share, err := r.svc.ShareAsset(ctx, ShareInput{AssetID: assetID, CustomerID: customerID, Title: "Price list", MaxDownloads: &limit})
	if err != nil {
		t.Fatal(err)
	}
	get := func(files FileStore) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.svc.Handler(files, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download/"+share.Token, nil))
		return rec
	}

	if rec := get(fakeFiles{err: errors.New("bucket down")}); rec.Code != http.StatusBadGateway {
		t.Fatalf("an unreadable file answered %d, want 502", rec.Code)
	}
	if rec := get(nil); rec.Code != http.StatusBadGateway {
		t.Fatalf("with no storage wired the share answered %d, want 502", rec.Code)
	}

	rec := get(fakeFiles{body: "%PDF the price list"})
	if rec.Code != http.StatusOK || rec.Body.String() != "%PDF the price list" || rec.Header().Get("Location") != "" {
		t.Fatalf("status %d body %q location %q, want the file itself", rec.Code, rec.Body.String(), rec.Header().Get("Location"))
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, "Price list 2027.pdf") {
		t.Errorf("Content-Disposition %q, want an attachment under the file's own name", cd)
	}
	if rec := get(fakeFiles{body: "again"}); rec.Code == http.StatusOK {
		t.Errorf("a share limited to one download was served twice, the two failed tries included: %d", rec.Code)
	}

	if _, err := r.conn.Exec(`DELETE FROM media_assets WHERE id = $1`, assetID); err != nil {
		t.Fatal(err)
	}
	if left, _ := r.svc.ListForCustomer(ctx, customerID); len(left) != 0 {
		t.Errorf("the file is gone but the customer still lists %d downloads of it", len(left))
	}
}

func TestSharesEndWhenTheCustomersAccountDoes(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for name, gone := range map[string]func(uuid.UUID) events.Event{
		"deleted": func(id uuid.UUID) events.Event { return &customer.CustomerDeleted{CustomerID: id} },
		"erased":  func(id uuid.UUID) events.Event { return &customer.CustomerErased{CustomerID: id} },
	} {
		assetID, leaving := r.fileAndCustomer(t)
		staying := uuid.New()
		theirs, err := r.svc.ShareAsset(ctx, ShareInput{AssetID: assetID, CustomerID: leaving, Title: "Price list"})
		if err != nil {
			t.Fatal(err)
		}
		kept, err := r.svc.ShareAsset(ctx, ShareInput{AssetID: assetID, CustomerID: staying, Title: "Price list"})
		if err != nil {
			t.Fatal(err)
		}
		bus := events.NewBus()
		r.svc.SubscribeToCustomerEvents(bus)
		bus.Publish(ctx, gone(leaving))
		if err := bus.WaitIdle(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := r.svc.Get(ctx, theirs.Token); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: the link of the account that is gone still resolves: %v", name, err)
		}
		if _, err := r.svc.Get(ctx, kept.Token); err != nil {
			t.Errorf("%s: another customer's share went too: %v", name, err)
		}
	}

	buyer := uuid.New()
	r.orders.state.CustomerID = &buyer
	r.orders.set("pending", order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	if err := r.svc.onCustomerGone(ctx, &customer.CustomerDeleted{CustomerID: buyer}); err != nil {
		t.Fatal(err)
	}
	if got := r.grants(t); len(got) != 1 || got[0].CustomerID == nil || *got[0].CustomerID != buyer {
		t.Errorf("a download the customer paid for went with the account: %+v", got)
	}
}

func TestADownloadSaysWhetherItWasBoughtOrShared(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	bus := events.NewBus()
	seen := make(chan *Accessed, 2)
	bus.Subscribe(EventAccessed, func(_ context.Context, e events.Event) error {
		seen <- e.(*Accessed)
		return nil
	})
	r.svc.bus = bus

	assetID, customerID := r.fileAndCustomer(t)
	share, err := r.svc.ShareAsset(ctx, ShareInput{AssetID: assetID, CustomerID: customerID, Title: "Price list"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Consume(ctx, share.Token); err != nil {
		t.Fatal(err)
	}
	r.orders.set("pending", order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	bought := r.grants(t)[0]
	if _, err := r.svc.Consume(ctx, bought.Token); err != nil {
		t.Fatal(err)
	}
	if err := bus.WaitIdle(ctx); err != nil {
		t.Fatal(err)
	}
	close(seen)
	for got := range seen {
		switch got.GrantID {
		case share.ID:
			if got.AssetID == nil || *got.AssetID != assetID || got.ProductID != nil {
				t.Errorf("the shared file's download: asset %v product %v", got.AssetID, got.ProductID)
			}
		case bought.ID:
			if got.ProductID == nil || *got.ProductID != bought.ProductID || got.AssetID != nil {
				t.Errorf("the bought file's download: asset %v product %v", got.AssetID, got.ProductID)
			}
		}
	}
}
