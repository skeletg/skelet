package download

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type lookup struct {
	mu    sync.Mutex
	state OrderDownloads
}

func (l *lookup) DownloadsForOrder(context.Context, uuid.UUID) (OrderDownloads, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state, nil
}

func (l *lookup) set(status, payment string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state.Status, l.state.PaymentStatus = status, payment
}

type rig struct {
	conn    *sql.DB
	svc     *Service
	orders  *lookup
	orderID uuid.UUID
}

func newRig(t *testing.T, files ...GrantableDownload) rig {
	t.Helper()
	conn := testdb.Require(t)
	var orderID uuid.UUID
	if err := conn.QueryRow(`INSERT INTO orders (channel_id, currency_code) SELECT id, currency_code FROM channels ORDER BY created_at LIMIT 1 RETURNING id`).Scan(&orderID); err != nil {
		t.Skipf("no channel to place an order on: %v", err)
	}
	if len(files) == 0 {
		files = []GrantableDownload{{ProductID: uuid.New(), DownloadID: uuid.New(), Title: "Manual", URL: "https://files.example.test/manual.pdf"}}
	}
	l := &lookup{state: OrderDownloads{Status: "pending", PaymentStatus: order.PaymentUnpaid, Email: "buyer@example.test", Items: files}}
	svc := NewService(NewRepository(db.New(conn)), txrunner.New(conn))
	svc.SetOrderLookup(l)
	return rig{conn: conn, svc: svc, orders: l, orderID: orderID}
}

func (r rig) grants(t *testing.T) []Grant {
	t.Helper()
	g, err := r.svc.ListForOrder(context.Background(), r.orderID)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func (r rig) payment(t *testing.T, status string) {
	t.Helper()
	if err := r.svc.onPaymentStatusChanged(context.Background(), &order.OrderPaymentStatusChanged{OrderID: r.orderID, Status: status}); err != nil {
		t.Fatal(err)
	}
}

// Cash on delivery and bank transfer place unpaid - handing over the file then gave it away for an order nobody has to pay
func TestAnUnpaidOrderGetsItsFilesOnlyOncePaymentIsSecured(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := r.svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: r.orderID}); err != nil {
		t.Fatal(err)
	}
	if n := len(r.grants(t)); n != 0 {
		t.Fatalf("an unpaid order was granted %d files at placement", n)
	}
	r.orders.set("pending", order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	if err := r.svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: r.orderID}); err != nil {
		t.Fatal(err)
	}
	if g := r.grants(t); len(g) != 1 || g[0].Email != "buyer@example.test" {
		t.Fatalf("grants after payment and two replays: %+v, want exactly one", g)
	}
}

func TestAnAuthorizedCardOrderIsGrantedAtPlacement(t *testing.T) {
	r := newRig(t)
	r.orders.set("pending", order.PaymentAuthorized)
	if err := r.svc.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: r.orderID}); err != nil {
		t.Fatal(err)
	}
	if n := len(r.grants(t)); n != 1 {
		t.Fatalf("%d grants, want the authorized order's file", n)
	}
}

// Two deliveries of the same event used to both read "no grants yet" and both mint a token
func TestRacingGrantsMintOneTokenPerFile(t *testing.T) {
	r := newRig(t, GrantableDownload{ProductID: uuid.New(), DownloadID: uuid.New(), Title: "A", URL: "https://x.test/a"},
		GrantableDownload{ProductID: uuid.New(), DownloadID: uuid.New(), Title: "B", URL: "https://x.test/b"})
	r.orders.set("pending", order.PaymentPaid)
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			if err := r.svc.grantForOrder(context.Background(), r.orderID); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := len(r.grants(t)); n != 2 {
		t.Fatalf("%d grants from six racing deliveries, want one per file", n)
	}
}

func TestACancelledVoidedOrRefundedOrderStopsServing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		revoke func(r rig) error
	}{
		{"cancelled", func(r rig) error {
			return r.svc.onOrderCancelled(context.Background(), &order.OrderCancelled{OrderID: r.orderID})
		}},
		{"voided", func(r rig) error {
			return r.svc.onPaymentStatusChanged(context.Background(), &order.OrderPaymentStatusChanged{OrderID: r.orderID, Status: order.PaymentVoided})
		}},
		{"refunded", func(r rig) error {
			return r.svc.onPaymentStatusChanged(context.Background(), &order.OrderPaymentStatusChanged{OrderID: r.orderID, Status: order.PaymentRefunded})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t)
			r.orders.set("pending", order.PaymentAuthorized)
			r.payment(t, order.PaymentAuthorized)
			token := r.grants(t)[0].Token
			if _, err := r.svc.Consume(context.Background(), token); err != nil {
				t.Fatalf("a live grant refused: %v", err)
			}
			if err := tc.revoke(r); err != nil {
				t.Fatal(err)
			}
			if _, err := r.svc.Consume(context.Background(), token); !errors.Is(err, ErrExpired) {
				t.Fatalf("a %s order's grant still served: %v", tc.name, err)
			}
		})
	}
}

// A payment that lands after the cancel - a late webhook - must not hand the files back out
func TestALatePaymentDoesNotRegrantACancelledOrder(t *testing.T) {
	r := newRig(t)
	r.orders.set(order.OrderStatusCancelled, order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	if n := len(r.grants(t)); n != 0 {
		t.Fatalf("a cancelled order was granted %d files", n)
	}
}

func TestConcurrentDownloadsCannotPassTheCap(t *testing.T) {
	limit := int32(3)
	r := newRig(t, GrantableDownload{ProductID: uuid.New(), DownloadID: uuid.New(), Title: "A", URL: "https://x.test/a", MaxDownloads: &limit})
	r.orders.set("pending", order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	token := r.grants(t)[0].Token
	var served atomic.Int32
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := r.svc.Consume(context.Background(), token); err == nil {
				served.Add(1)
			} else if !errors.Is(err, ErrLimitReached) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if served.Load() != limit {
		t.Fatalf("%d downloads served against a cap of %d", served.Load(), limit)
	}
}

type fakeFiles struct {
	prefix string
	body   string
	err    error
}

func (f fakeFiles) KeyFor(u string) (string, bool) { return strings.CutPrefix(u, f.prefix) }

func (f fakeFiles) Open(context.Context, string) (io.ReadCloser, int64, string, error) {
	if f.err != nil {
		return nil, 0, "", f.err
	}
	return io.NopCloser(strings.NewReader(f.body)), int64(len(f.body)), "application/pdf", nil
}

// A redirect to the file hands the buyer its URL, after which neither the token nor the cap matters
func TestAStoredFileStreamsWithoutRevealingItsURL(t *testing.T) {
	r := newRig(t, GrantableDownload{ProductID: uuid.New(), DownloadID: uuid.New(), Title: "Book", URL: "https://cdn.example.test/media/books/book.pdf"})
	r.orders.set("pending", order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	token := r.grants(t)[0].Token
	files := fakeFiles{prefix: "https://cdn.example.test/media/", body: "%PDF-1.7 the book"}

	rec := httptest.NewRecorder()
	r.svc.Handler(files, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download/"+token, nil))
	if rec.Code != http.StatusOK || rec.Body.String() != files.body {
		t.Fatalf("status %d body %q, want the file itself", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("the response pointed at %q", loc)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, "book.pdf") {
		t.Errorf("Content-Disposition %q, want an attachment named after the file", cd)
	}
	if r.grants(t)[0].DownloadsUsed != 1 {
		t.Error("a streamed download did not count as a use")
	}

	rec = httptest.NewRecorder()
	r.svc.Handler(fakeFiles{prefix: files.prefix, err: errors.New("bucket down")}, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download/"+token, nil))
	if rec.Code != http.StatusBadGateway || r.grants(t)[0].DownloadsUsed != 1 {
		t.Fatalf("an unreadable file answered %d and left %d uses spent, want 502 and the use kept", rec.Code, r.grants(t)[0].DownloadsUsed)
	}
}

func TestAnExternalFileStillRedirects(t *testing.T) {
	r := newRig(t)
	r.orders.set("pending", order.PaymentPaid)
	r.payment(t, order.PaymentPaid)
	rec := httptest.NewRecorder()
	r.svc.Handler(fakeFiles{prefix: "https://cdn.example.test/"}, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download/"+r.grants(t)[0].Token, nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "https://files.example.test/manual.pdf" {
		t.Fatalf("status %d location %q, want a redirect to the external file", rec.Code, rec.Header().Get("Location"))
	}
}

// A cash deposit lands as partially_captured - a tenth of the price must not unlock the whole file
func TestAPartialDepositDoesNotUnlockTheFile(t *testing.T) {
	r := newRig(t)
	r.orders.set("pending", order.PaymentPartiallyCaptured)
	r.payment(t, order.PaymentPartiallyCaptured)
	if n := len(r.grants(t)); n != 0 {
		t.Fatalf("a deposit unlocked %d files", n)
	}
}
