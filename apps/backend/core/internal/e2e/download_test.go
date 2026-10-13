package e2e

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// addDownloadable attaches one downloadable file to the store's product - maxDownloads nil is unlimited
func (h *harness) addDownloadable(t *testing.T, productID uuid.UUID, title string, maxDownloads *int32) catalog.ProductDownload {
	t.Helper()
	d, err := h.D.Catalog.AddProductDownload(h.Ctx, catalog.AddProductDownloadInput{
		ProductID:    productID,
		Title:        title,
		URL:          "https://cdn.invalid/" + uuid.NewString()[:8] + ".zip",
		MaxDownloads: maxDownloads,
	})
	if err != nil {
		t.Fatalf("add product download: %v", err)
	}
	return d
}

// Records the cash on delivery in full - a COD order is unpaid until then, and its files wait for it
func (h *harness) collectCOD(t *testing.T, orderID uuid.UUID) {
	t.Helper()
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{OrderID: orderID, Method: "cod", Reference: "e2e-cod"}); err != nil {
		t.Fatalf("record cod payment: %v", err)
	}
	h.Settle(t)
}

func (h *harness) grantsFor(t *testing.T, orderID uuid.UUID) []download.Grant {
	t.Helper()
	got, err := h.D.Download.ListForOrder(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("list grants: %v", err)
	}
	return got
}

// Scenario: buying a digital product grants download access once the order is paid, once per downloadable file, and the grant carries the buyer's identity so the account page can list it
// Cash on delivery places unpaid - granting at placement handed the file over for an order nobody had to pay
func TestDigitalPurchaseGrantsDownloadAccess(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	file := h.addDownloadable(t, f.Product.ID, "Manual PDF", nil)

	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	if got := h.grantsFor(t, o.ID); len(got) != 0 {
		t.Fatalf("%d grants for an unpaid cash-on-delivery order, want none until the cash is in", len(got))
	}
	h.collectCOD(t, o.ID)

	grants := h.grantsFor(t, o.ID)
	if len(grants) != 1 {
		t.Fatalf("%d grants, want one per downloadable file", len(grants))
	}
	g := grants[0]
	if g.Title != "Manual PDF" {
		t.Errorf("grant title = %q, want the download's title", g.Title)
	}
	if g.ProductID != f.Product.ID || g.DownloadID != file.ID {
		t.Errorf("grant points at product %s file %s, want %s / %s", g.ProductID, g.DownloadID, f.Product.ID, file.ID)
	}
	if g.Token == "" {
		t.Error("grant carries no token, so there is nothing to serve")
	}
	if g.OrderID == nil || *g.OrderID != o.ID {
		t.Errorf("grant order = %v, want %s", g.OrderID, o.ID)
	}
	if g.DownloadsUsed != 0 {
		t.Errorf("downloads used = %d on a fresh grant, want 0", g.DownloadsUsed)
	}
}

// Two files on one product means two grants - collapsing them would leave one file unreachable
func TestDigitalPurchaseGrantsOneTokenPerFile(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.addDownloadable(t, f.Product.ID, "Manual", nil)
	h.addDownloadable(t, f.Product.ID, "Bonus track", nil)

	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)

	grants := h.grantsFor(t, o.ID)
	if len(grants) != 2 {
		t.Fatalf("%d grants, want 2", len(grants))
	}
	if grants[0].Token == grants[1].Token {
		t.Error("both files share one token, so one download can exhaust the other")
	}
	titles := map[string]bool{grants[0].Title: true, grants[1].Title: true}
	for _, want := range []string{"Manual", "Bonus track"} {
		if !titles[want] {
			t.Errorf("no grant for %q", want)
		}
	}
}

// OrderPlaced is at-least-once, so a replay must not mint a second set of tokens - each one is independently spendable against the cap
func TestDigitalGrantsAreIdempotentUnderReplay(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.addDownloadable(t, f.Product.ID, "Manual", nil)

	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)
	first := h.grantsFor(t, o.ID)
	if len(first) != 1 {
		t.Fatalf("%d grants after placement, want 1", len(first))
	}

	h.RearmOutbox(t, "order.placed", o.ID.String(), 1)
	h.Settle(t)

	after := h.grantsFor(t, o.ID)
	if len(after) != 1 {
		t.Errorf("%d grants after a replayed placement, want still 1", len(after))
	}
	if after[0].Token != first[0].Token {
		t.Error("the replay re-minted the token, so the original link would stop working")
	}
}

// A non-digital order must not mint tokens - a grant on a physical product is a free download
func TestNonDigitalPurchaseGrantsNothing(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500) // no downloadable attached

	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)

	if got := h.grantsFor(t, o.ID); len(got) != 0 {
		t.Errorf("%d grants for an order with no downloadable lines, want 0", len(got))
	}
}

// Consuming is what the /download/:token route does - it increments the use count under a row lock so a capped file can't be over-served
func TestDownloadConsumeIncrementsTheUseCount(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.addDownloadable(t, f.Product.ID, "Manual", nil)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)
	token := h.grantsFor(t, o.ID)[0].Token

	for want := int32(1); want <= 3; want++ {
		got, err := h.D.Download.Consume(h.Ctx, token)
		if err != nil {
			t.Fatalf("consume %d: %v", want, err)
		}
		if got.DownloadsUsed != want {
			t.Errorf("downloads used after consume %d = %d, want %d", want, got.DownloadsUsed, want)
		}
	}
	// the stored row agrees, not just the returned copy
	reread, err := h.D.Download.Get(h.Ctx, token)
	if err != nil {
		t.Fatalf("re-read grant: %v", err)
	}
	if reread.DownloadsUsed != 3 {
		t.Errorf("stored downloads used = %d, want 3", reread.DownloadsUsed)
	}
	h.Settle(t)
}

// The cap is what makes a paid download finite - the (max+1)th attempt has to refuse and must not increment past the limit
func TestDownloadConsumeEnforcesTheCap(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	max := int32(2)
	h.addDownloadable(t, f.Product.ID, "Capped", &max)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)
	token := h.grantsFor(t, o.ID)[0].Token

	for i := range 2 {
		if _, err := h.D.Download.Consume(h.Ctx, token); err != nil {
			t.Fatalf("consume %d within the cap: %v", i+1, err)
		}
	}
	_, err := h.D.Download.Consume(h.Ctx, token)
	if !errors.Is(err, download.ErrLimitReached) {
		t.Errorf("consume past the cap = %v, want ErrLimitReached", err)
	}
	reread, err := h.D.Download.Get(h.Ctx, token)
	if err != nil {
		t.Fatalf("re-read grant: %v", err)
	}
	if reread.DownloadsUsed != 2 {
		t.Errorf("downloads used = %d after the refused attempt, want it capped at 2", reread.DownloadsUsed)
	}
	h.Settle(t)
}

// An unknown token must not resolve to anyone's file - this is the whole access control on the route
func TestDownloadConsumeRefusesAnUnknownToken(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.addDownloadable(t, f.Product.ID, "Manual", nil)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)
	real := h.grantsFor(t, o.ID)[0].Token

	for name, token := range map[string]string{
		"empty":     "",
		"garbage":   "not-a-token",
		"truncated": real[:len(real)-1],
	} {
		if _, err := h.D.Download.Consume(h.Ctx, token); err == nil {
			t.Errorf("consuming a %s token succeeded", name)
		}
	}
	// and the real grant is untouched by the failed attempts
	reread, err := h.D.Download.Get(h.Ctx, real)
	if err != nil {
		t.Fatalf("re-read grant: %v", err)
	}
	if reread.DownloadsUsed != 0 {
		t.Errorf("downloads used = %d after only failed attempts, want 0", reread.DownloadsUsed)
	}
	h.Settle(t)
}

// Concurrent clicks on the same capped link must not exceed the cap - the row lock is the guard, and this is the race a plain read-then-write would lose
func TestDownloadConsumeRaceNeverExceedsTheCap(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	max := int32(3)
	h.addDownloadable(t, f.Product.ID, "Capped", &max)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)
	token := h.grantsFor(t, o.ID)[0].Token

	const attempts = 8
	errs := make(chan error, attempts)
	for range attempts {
		go func() {
			_, err := h.D.Download.Consume(h.Ctx, token)
			errs <- err
		}()
	}
	var ok int
	for range attempts {
		if err := <-errs; err == nil {
			ok++
		}
	}

	if ok != int(max) {
		t.Errorf("%d of %d concurrent consumes succeeded, want exactly the cap of %d", ok, attempts, max)
	}
	reread, err := h.D.Download.Get(h.Ctx, token)
	if err != nil {
		t.Fatalf("re-read grant: %v", err)
	}
	if reread.DownloadsUsed != max {
		t.Errorf("downloads used = %d, want the cap of %d", reread.DownloadsUsed, max)
	}
	h.Settle(t)
}

// A registered buyer's grants have to be listable by customer, which is what the account page renders
func TestDownloadGrantsAreListableByCustomer(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.addDownloadable(t, f.Product.ID, "Manual", nil)

	email := "dl-" + uuid.NewString()[:8] + "@e2e.test"
	c, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, email, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	o := h.placeFor(t, f, c.ID, email, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)

	mine, err := h.D.Download.ListForCustomer(h.Ctx, c.ID)
	if err != nil {
		t.Fatalf("ListForCustomer: %v", err)
	}
	var found bool
	for _, g := range mine {
		if g.OrderID != nil && *g.OrderID == o.ID {
			found = true
			if g.CustomerID == nil || *g.CustomerID != c.ID {
				t.Errorf("grant customer = %v, want %s", g.CustomerID, c.ID)
			}
		}
	}
	if !found {
		t.Errorf("the order's grant is missing from the customer's %d grants", len(mine))
	}
}

// Cancelling a paid digital order takes the files back - its grant stops serving
func TestCancellingADigitalOrderRevokesItsDownloads(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.addDownloadable(t, f.Product.ID, "Manual", nil)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.collectCOD(t, o.ID)
	token := h.grantsFor(t, o.ID)[0].Token

	h.Cancel(t, o.ID)
	h.Settle(t)
	if _, err := h.D.Download.Consume(h.Ctx, token); !errors.Is(err, download.ErrExpired) {
		t.Fatalf("a cancelled order's download = %v, want ErrExpired", err)
	}
}
