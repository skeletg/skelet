package order

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	shippingpkg "github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

// "forbidden" for someone else's order and "not found" for a missing one told a caller which order ids exist
func TestSomeoneElsesOrderReadsAsMissing(t *testing.T) {
	conn := statusTestDB(t)
	svc := moneySvc(conn)
	ctx := context.Background()
	orderID, _ := seedPaidOrder(t, conn, OrderStatusConfirmed, PaymentUnpaid, 0)
	if _, err := svc.GetOrderForCustomer(ctx, uuid.New(), orderID); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("someone else's order = %v, want ErrOrderNotFound", err)
	}
	if _, err := svc.GetOrderForCustomer(ctx, uuid.New(), uuid.New()); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("a missing order = %v, want ErrOrderNotFound", err)
	}
	if _, err := svc.HoldOrder(ctx, uuid.New(), ""); !errors.Is(err, ErrOrderNotFound) {
		t.Errorf("holding a missing order = %v, want ErrOrderNotFound", err)
	}
	if _, err := svc.UpdateStatus(ctx, orderID, OrderStatusFulfilled); conflictCode(err) != "ORDER_INVALID_TRANSITION" {
		t.Errorf("confirmed -> fulfilled = %v, want ORDER_INVALID_TRANSITION", err)
	}
}

// A label listed every order line - a partial shipment's label declared goods that weren't in the box - and every call bought another one
func TestLabelsDescribeTheShipmentAndAreBoughtOnce(t *testing.T) {
	conn := statusTestDB(t)
	svc := moneySvc(conn)
	ctx := context.Background()
	orderID, lineID := seedPaidOrder(t, conn, OrderStatusProcessing, PaymentPaid, 0)
	var otherLine uuid.UUID
	if err := conn.QueryRow(`INSERT INTO order_lines (order_id, product_name, product_sku, unit_price, quantity, total_price)
		VALUES ($1, 'other', 'MP-2', 500, 1, 500) RETURNING id`, orderID).Scan(&otherLine); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM order_fulfillments WHERE order_id = $1`, orderID) })
	if _, err := svc.CreateFulfillment(ctx, CreateFulfillmentInput{OrderID: orderID, Lines: []FulfillmentLineInput{{OrderLineID: lineID, Quantity: 1}}}); err != nil {
		t.Fatal(err)
	}
	fs, err := svc.repo.ListFulfillments(ctx, orderID)
	if err != nil || len(fs) != 1 {
		t.Fatalf("fulfillments %v (%v)", fs, err)
	}
	o, err := svc.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		t.Fatal(err)
	}
	_ = o
	carrier := &labelCarrier{}
	slug := "test-labels-" + uuid.NewString()[:8]
	shippingpkg.RegisterCarrier(shippingpkg.CarrierRegistration{Slug: slug, Carrier: carrier})
	svc.shipping = shippingpkg.NewService(shippingpkg.NewRepository(dbsqlc.New(conn)))
	if _, err := conn.Exec(`UPDATE orders SET shipping_carrier_slug = $2 WHERE id = $1`, orderID, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GenerateLabel(ctx, fs[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if items := carrier.req.Items; len(items) != 1 || items[0].SKU != "MP-1" || items[0].Quantity != 1 {
		t.Fatalf("label declared %+v, want the one unit of MP-1 in the box", items)
	}
	if _, err := svc.GenerateLabel(ctx, fs[0].ID, false); conflictCode(err) != "LABEL_EXISTS" {
		t.Fatalf("a second label without force = %v, want LABEL_EXISTS", err)
	}
	if _, err := svc.GenerateLabel(ctx, fs[0].ID, true); err != nil {
		t.Fatalf("a forced second label = %v", err)
	}
	if carrier.calls != 2 {
		t.Errorf("bought %d labels, want 2 - the refused one must not reach the carrier", carrier.calls)
	}
	if _, err := conn.Exec(`UPDATE order_fulfillments SET status = 'cancelled' WHERE id = $1`, fs[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GenerateLabel(ctx, fs[0].ID, true); conflictCode(err) != "LABEL_FULFILLMENT_CANCELLED" {
		t.Fatalf("a label for a cancelled shipment = %v, want LABEL_FULFILLMENT_CANCELLED", err)
	}
}

type failClosedTax struct{}

func (failClosedTax) Tax(context.Context, RemoteTaxInput) (int64, bool, error) {
	return 0, false, errors.New("tax provider down")
}

// The estimate quoted core tax when a fail-closed provider was down - checkout then refused the order it had just priced
func TestTheEstimateFailsWhereCheckoutWould(t *testing.T) {
	svc := &Service{remoteTax: failClosedTax{}}
	if _, err := svc.EstimateTax(context.Background(), TaxEstimateInput{Store: &channel.Channel{CurrencyCode: "USD"}}); err == nil {
		t.Fatal("the estimate quoted a tax checkout would refuse")
	}
}

func TestPolicyReasonsAreCapped(t *testing.T) {
	long := strings.Repeat("ž", maxPolicyReasonRunes+50)
	got := capPolicyReason(long)
	if utf8.RuneCountInString(got) != maxPolicyReasonRunes+1 || !utf8.ValidString(got) {
		t.Fatalf("capped reason has %d runes", utf8.RuneCountInString(got))
	}
	if capPolicyReason("short") != "short" {
		t.Fatal("a short reason was cut")
	}
}

type labelCarrier struct {
	req   shippingpkg.LabelRequest
	calls int
}

func (c *labelCarrier) Quote(context.Context, shippingpkg.RateRequest) ([]shippingpkg.RatedMethod, error) {
	return nil, nil
}

func (c *labelCarrier) Label(_ context.Context, req shippingpkg.LabelRequest) (shippingpkg.LabelResult, error) {
	c.req = req
	c.calls++
	return shippingpkg.LabelResult{Format: "pdf", Doc: []byte("%PDF"), TrackingNumber: "TRK"}, nil
}
