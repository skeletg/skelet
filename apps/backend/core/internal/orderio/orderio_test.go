package orderio

import (
	"errors"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
)

func TestADateOnlyEndCoversThatWholeDay(t *testing.T) {
	end, err := parseBound("2026-03-01", time.Time{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC); !end.Equal(want) {
		t.Fatalf("end = %v, want the next midnight %v as the exclusive bound", end, want)
	}
	start, _ := parseBound("2026-03-01", time.Time{}, false)
	if want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("start = %v, want %v", start, want)
	}
	instant := "2026-03-01T15:04:05Z"
	if got, _ := parseBound(instant, time.Time{}, true); got.Format(time.RFC3339) != instant {
		t.Fatalf("an RFC 3339 end = %v, want it used as given", got)
	}
}

func TestMalformedOptionsAreRefusedBeforeTheRunQueues(t *testing.T) {
	reg := dataio.NewRegistry()
	reg.Register(OrderLineResource(nil))
	for name, vals := range map[string]map[string]string{
		"start":  {optStartDate: "01/03/2026"},
		"end":    {optEndDate: "yesterday"},
		"status": {optStatuses: "pending, shipped"},
	} {
		err := reg.ValidateOptions("order_lines", dataio.NewOpts(vals), dataio.DirectionExport)
		if !errors.Is(err, dataio.ErrInvalidOption) {
			t.Errorf("%s: err = %v, want ErrInvalidOption", name, err)
		}
	}
	ok := map[string]string{optStartDate: "2026-03-01", optEndDate: "2026-03-01T10:00:00Z", optStatuses: "Pending, on_hold"}
	if err := reg.ValidateOptions("order_lines", dataio.NewOpts(ok), dataio.DirectionExport); err != nil {
		t.Fatalf("valid options refused: %v", err)
	}
	if _, err := newExporter(nil, dataio.NewOpts(map[string]string{optStatuses: "shipped"}), time.Now()); !errors.Is(err, errBadStatus) {
		t.Fatalf("exporter built over an unknown status: %v", err)
	}
	if _, err := newExporter(nil, dataio.NewOpts(map[string]string{optEndDate: "2026-13-01"}), time.Now()); !errors.Is(err, errBadDate) {
		t.Fatalf("exporter built over a bad date: %v", err)
	}
}

func TestOrderLinesNeedTheCustomerGrantToo(t *testing.T) {
	reg := dataio.NewRegistry()
	reg.Register(OrderLineResource(nil))
	res, err := reg.Get("order_lines")
	if err != nil {
		t.Fatal(err)
	}
	perms := res.ViewPermissions()
	if len(perms) != 2 || perms[0] != "orders.view" || perms[1] != "customers.manage" {
		t.Fatalf("view permissions = %v, want orders.view and customers.manage", perms)
	}
	if res.CanImport() {
		t.Fatal("order lines must not be importable - a spreadsheet would invent revenue")
	}
}

func TestColumnNamesStayAccountingStable(t *testing.T) {
	have := map[string]bool{}
	for _, f := range orderFields {
		have[f.Name] = true
	}
	for _, must := range []string{"order_number", "sku", "quantity", "unit_price_cents", "line_total_cents", "order_total_cents", "currency"} {
		if !have[must] {
			t.Errorf("order columns are missing %q", must)
		}
	}
}

// A From after the To exported an empty file with nothing to say why
func TestAnInvertedRangeIsRefused(t *testing.T) {
	inverted := map[string]string{optStartDate: "2026-03-05", optEndDate: "2026-03-01"}
	if _, err := newExporter(nil, dataio.NewOpts(inverted), time.Now()); !errors.Is(err, errBadRange) {
		t.Fatalf("From after To = %v, want errBadRange", err)
	}
	sameDay := map[string]string{optStartDate: "2026-03-01", optEndDate: "2026-03-01"}
	if _, err := newExporter(nil, dataio.NewOpts(sameDay), time.Now()); err != nil {
		t.Fatalf("a single day = %v, want it accepted", err)
	}
}
