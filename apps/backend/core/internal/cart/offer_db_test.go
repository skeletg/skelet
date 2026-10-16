package cart

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/outbox"
)

func (r dbRig) offer(t *testing.T) Cart {
	t.Helper()
	c, err := r.svc.AdminCreateOffer(context.Background(), CreateOfferInput{ChannelID: r.channel, Email: "offer@example.test", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM carts WHERE id = $1`, c.ID) })
	return c
}

func TestUnlockedOfferProtectsSpecialLines(t *testing.T) {
	r := newDBRig(t, catalogStub{prices: map[string]SimplePricing{"sku": {ProductName: "Item", UnitPrice: 1000}}})
	c := r.offer(t)
	ctx := WithOfferGrant(context.Background(), OfferGrant{CartID: c.ID, LinkVersion: c.Offer.LinkVersion})
	c, err := r.svc.AdminAddToCart(ctx, c.ID, "sku", 1)
	if err != nil {
		t.Fatal(err)
	}
	unit := int64(500)
	if _, err := r.svc.AdminSetLinePrice(ctx, c.ID, c.Lines[0].ID, &unit); err != nil {
		t.Fatal(err)
	}
	c, err = r.svc.AdminAddCustomLine(ctx, c.ID, CustomLineInput{Name: "Fee", UnitPrice: 200, Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range c.Lines {
		for name, edit := range map[string]func() error{
			"quantity":       func() error { _, err := r.svc.UpdateCartLine(ctx, c.ID, line.ID, nil, 2); return err },
			"remove":         func() error { _, err := r.svc.RemoveFromCart(ctx, c.ID, line.ID, nil); return err },
			"save for later": func() error { _, err := r.svc.SaveForLater(ctx, c.ID, line.ID, nil); return err },
			"clear":          func() error { return r.svc.ClearCart(ctx, c.ID, nil) },
			"add":            func() error { _, err := r.svc.AddToCart(ctx, c.ID, nil, line.ProductSku, 1); return err },
			"configured add": func() error {
				_, err := r.svc.AddConfiguredToCart(ctx, c.ID, nil, AddConfiguredToCartInput{ProductSku: line.ProductSku, Quantity: 1})
				return err
			},
			"bundle add": func() error {
				_, err := r.svc.AddBundleToCart(ctx, c.ID, nil, AddBundleToCartInput{ProductSku: line.ProductSku, Quantity: 1})
				return err
			},
		} {
			if err := edit(); !errors.Is(err, ErrOfferLocked) {
				t.Errorf("%s %s: %v, want locked", name, line.ProductSku, err)
			}
		}
		if _, err := r.svc.AdminUpdateCartLine(ctx, c.ID, line.ID, 2); err != nil {
			t.Fatalf("admin quantity: %v", err)
		}
		if line.IsCustom {
			if _, err := r.svc.AdminUpdateCartLine(ctx, c.ID, line.ID, 1.5); !errors.Is(err, ErrQuantityInvalid) {
				t.Fatalf("fractional edit: %v", err)
			}
		}
	}
	for _, qty := range []float64{0, -1, 0.5, math.NaN(), math.Inf(1)} {
		if _, err := r.svc.AdminAddCustomLine(ctx, c.ID, CustomLineInput{Name: "Fraction", Quantity: qty}); !errors.Is(err, ErrQuantityInvalid) {
			t.Errorf("quantity %v: %v", qty, err)
		}
	}
	reread, err := r.svc.GetCart(ctx, c.ID, nil)
	if err != nil || len(reread.Lines) != 2 {
		t.Fatalf("lines changed: %+v, %v", reread.Lines, err)
	}
	for _, line := range reread.Lines {
		if line.Quantity != 2 {
			t.Errorf("quantity = %v, want admin's 2", line.Quantity)
		}
	}
	if _, err := r.svc.AdminDuplicateCart(ctx, c.ID, nil); !errors.Is(err, ErrOfferLocked) {
		t.Fatalf("duplicate offer: %v", err)
	}
	// An unlocked ordinary line still behaves as a shopper line.
	if _, err := r.svc.AdminSetLinePrice(ctx, c.ID, lineBySku(t, reread, "sku").ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AddToCart(ctx, c.ID, nil, "sku", 1); err != nil {
		t.Fatalf("ordinary add: %v", err)
	}
}

type failOfferCheckoutRepository struct{ Repository }

func (r failOfferCheckoutRepository) WithTx(tx *sql.Tx) Repository {
	return failOfferCheckoutRepository{r.Repository.WithTx(tx)}
}
func (r failOfferCheckoutRepository) SetCheckout(context.Context, uuid.UUID, Checkout) error {
	return errors.New("checkout write failed")
}

func TestOfferSettingsAndCheckoutCommitTogether(t *testing.T) {
	r := newDBRig(t, catalogStub{})
	c := r.offer(t)
	ctx := context.Background()
	settings := OfferSettings{ExpiresAt: time.Now().Add(2 * time.Hour), Message: "Updated"}
	apply := func(c *Cart) error { c.Checkout.ContactPhone = "+12025550123"; return nil }
	for _, mutate := range []func(*OfferSettings){
		func(s *OfferSettings) { s.Message = strings.Repeat("x", maxOfferMessageRunes+1) },
		func(s *OfferSettings) { s.ShowInAccount = true },
		func(s *OfferSettings) { s.ExpiresAt = time.Now().Add(-time.Hour) },
	} {
		bad := settings
		mutate(&bad)
		if err := r.svc.AdminUpdateOffer(ctx, c.ID, bad, false, apply); err == nil {
			t.Fatal("invalid save accepted")
		}
	}
	original := r.svc.repo
	r.svc.repo = failOfferCheckoutRepository{original}
	if err := r.svc.AdminUpdateOffer(ctx, c.ID, settings, false, apply); err == nil {
		t.Fatal("failed checkout saved")
	}
	r.svc.repo = original
	got, err := r.svc.GetCartByIDReadOnly(ctx, c.ID)
	if err != nil || got.Checkout.ContactPhone != "" || got.Offer.Message != "" || !got.Offer.ExpiresAt.Equal(c.Offer.ExpiresAt) {
		t.Fatalf("partial save: %+v, %v", got, err)
	}
	if err := r.svc.AdminUpdateOffer(ctx, c.ID, settings, false, apply); err != nil {
		t.Fatal(err)
	}
	got, err = r.svc.GetCartByIDReadOnly(ctx, c.ID)
	if err != nil || got.Checkout.ContactPhone != "+12025550123" || got.Offer.Message != "Updated" {
		t.Fatalf("save: %+v, %v", got, err)
	}
}

type failingOfferOutbox struct{}

func (failingOfferOutbox) Enqueue(context.Context, *sql.Tx, events.Event) error {
	return errors.New("outbox unavailable")
}

func TestOfferStampsAndEventsCommitTogether(t *testing.T) {
	r := newDBRig(t, catalogStub{})
	c := r.offer(t)
	ctx := context.Background()
	o := outbox.New(db.New(r.conn), events.NewBus())
	RegisterOutboxTypes(o)
	r.svc.SetOutbox(failingOfferOutbox{})
	if err := r.svc.MarkOfferSent(ctx, c.ID, c.Email); err == nil {
		t.Fatal("failed enqueue accepted")
	}
	r.svc.RecordOfferOpened(ctx, &c)
	got, err := r.svc.GetCartByIDReadOnly(ctx, c.ID)
	if err != nil || got.Offer.SentAt != nil || got.Offer.OpenedAt != nil || c.Offer.OpenedAt != nil {
		t.Fatalf("stamps survived rollback: %+v, %v", got.Offer, err)
	}
	r.svc.SetOutbox(o)
	r.svc.SetOfferLinks(NewOfferLinks(r.svc.repo))
	if _, err := r.svc.OpenOffer(ctx, c.Offer.LinkToken, uuid.New(), nil); !errors.Is(err, ErrOfferLinkInvalid) {
		t.Fatalf("wrong channel: %v", err)
	}
	got, err = r.svc.GetCartByIDReadOnly(ctx, c.ID)
	if err != nil || got.Offer.OpenedAt != nil {
		t.Fatalf("wrong-channel open stamped: %+v, %v", got.Offer, err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { copy := c; copyOffer := *c.Offer; copy.Offer = &copyOffer; r.svc.RecordOfferOpened(ctx, &copy) })
	}
	wg.Wait()
	count := func(typ string, want int) {
		t.Helper()
		var n int
		if err := r.conn.QueryRow(`SELECT count(*) FROM event_outbox WHERE event_type = $1 AND payload->>'CartID' = $2`, typ, c.ID.String()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("%s rows = %d, want %d", typ, n, want)
		}
	}
	count(EventOfferOpened, 1)
	if err := r.svc.MarkOfferSent(ctx, c.ID, c.Email); err != nil {
		t.Fatal(err)
	}
	count(EventOfferSent, 1)
	expire := func() {
		t.Helper()
		if _, err := r.conn.Exec(`UPDATE carts SET offer_expires_at = clock_timestamp() - interval '1 minute' WHERE id = $1`, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	expire()
	r.svc.SetOutbox(failingOfferOutbox{})
	if _, err := r.svc.AnnounceExpiredOffers(ctx, 100); err == nil {
		t.Fatal("failed expiration enqueue accepted")
	}
	r.svc.SetOutbox(o)
	for range 8 {
		wg.Go(func() {
			if _, err := r.svc.AnnounceExpiredOffers(ctx, 100); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	count(EventOfferExpired, 1)
	if _, err := r.svc.AdminSetOffer(ctx, c.ID, OfferSettings{ExpiresAt: time.Now().Add(time.Hour)}, false); err != nil {
		t.Fatal(err)
	}
	expire()
	if _, err := r.svc.AnnounceExpiredOffers(ctx, 100); err != nil {
		t.Fatal(err)
	}
	count(EventOfferExpired, 2)
}
