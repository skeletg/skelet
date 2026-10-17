package wiring

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

type oneOrder struct{ o order.Order }

func (f oneOrder) GetByID(context.Context, uuid.UUID) (order.Order, error) { return f.o, nil }

type flakyCatalog struct{ err error }

func (f flakyCatalog) GetProductBySKU(context.Context, string) (catalog.Product, error) {
	return catalog.Product{}, f.err
}
func (f flakyCatalog) ProductKindBySKU(context.Context, string) (string, error) { return "", f.err }
func (f flakyCatalog) ListProductDownloads(context.Context, uuid.UUID) ([]catalog.ProductDownload, error) {
	return nil, f.err
}
func (f flakyCatalog) GetSubscriptionTerms(context.Context, uuid.UUID) (catalog.SubscriptionTerms, error) {
	return catalog.SubscriptionTerms{}, f.err
}

// A database blip used to drop the line and report success, so the outbox never retried and the customer never got what they paid for
func TestOrderIssuersRetryATransientCatalogError(t *testing.T) {
	o := oneOrder{order.Order{Lines: []order.OrderLine{{ProductSku: "GIFT-50"}}}}
	for name, tc := range map[string]struct {
		err     error
		wantErr bool
	}{
		"database down":   {errors.New("connection refused"), true},
		"sku not on file": {catalog.ErrProductNotFound, false},
	} {
		t.Run(name, func(t *testing.T) {
			cat := flakyCatalog{err: tc.err}
			_, gerr := GiftcardOrderLookup{orderSvc: o, catalogSvc: cat}.GiftcardLines(context.Background(), uuid.New())
			_, derr := DownloadOrderLookup{orderSvc: o, catalogSvc: cat}.DownloadsForOrder(context.Background(), uuid.New())
			_, serr := SubscriptionOrderLookup{orderSvc: o, catalogSvc: cat}.SubscriptionsForOrder(context.Background(), uuid.New())
			for which, err := range map[string]error{"giftcard": gerr, "download": derr, "subscription": serr} {
				if (err != nil) != tc.wantErr {
					t.Errorf("%s: err = %v, want error %v", which, err, tc.wantErr)
				}
			}
		})
	}
}

func TestADownloadNameSurvivesTheHeader(t *testing.T) {
	_, params, err := mime.ParseMediaType(attachment("narudžbine.csv"))
	if err != nil || params["filename"] != "narudžbine.csv" {
		t.Fatalf("Content-Disposition round-trip = %v, %v", params, err)
	}
}

type countingSuppress struct{ calls int }

func (c *countingSuppress) Suppress(context.Context, string, string, string) error {
	c.calls++
	return nil
}
func (c *countingSuppress) RecordSoftBounce(context.Context, string, string, int) (bool, int, error) {
	c.calls++
	return false, 0, nil
}

// With no suite there is no suppression list, and a 503 would have the provider redeliver forever
func TestTheEmailWebhookIsOffWithoutSignals(t *testing.T) {
	sup := &countingSuppress{}
	h := emailEventsHandler("tok", false, nil, sup)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/webhooks/email/events", strings.NewReader(`{"type":"complaint","email":"a@b.test"}`))
	r.Header.Set("Authorization", "Bearer tok")
	h(w, r)
	if w.Code != http.StatusNotFound || sup.calls != 0 {
		t.Fatalf("status %d after %d suppression calls, want a 404 and none", w.Code, sup.calls)
	}
}
