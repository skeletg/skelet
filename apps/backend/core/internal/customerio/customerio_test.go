package customerio

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

// A nil customer service means any row surviving validation panics, which is how these prove a row never reached a write
func rejectRow(t *testing.T, cells map[string]string) error {
	t.Helper()
	im := &customerImporter{opts: optsFor(uuid.New())}
	r := dataio.NewRow()
	for k, v := range cells {
		r.Set(k, v)
	}
	_, err := im.Row(context.Background(), r)
	if err == nil {
		err = r.Err()
	}
	return err
}

func optsFor(channel uuid.UUID) dataio.Opts {
	o := dataio.NewOpts(nil)
	o.ChannelID = channel
	return o
}

func TestImportRejectsMalformedEmail(t *testing.T) {
	for _, bad := range []string{"", "no-at-sign", "   "} {
		if err := rejectRow(t, map[string]string{colEmail: bad}); err == nil {
			t.Errorf("email %q was accepted, want a row error", bad)
		}
	}
}

// Customers are per-channel, so importing without one would silently write into whichever channel the query defaulted to
func TestImportRefusesWithoutAChannel(t *testing.T) {
	im := &customerImporter{opts: optsFor(uuid.Nil)}
	r := dataio.NewRow()
	r.Set(colEmail, "ada@example.com")
	if _, err := im.Row(context.Background(), r); err == nil {
		t.Fatal("want a refusal when no channel is selected")
	}
}

func TestImportLowercasesTheEmailForMatching(t *testing.T) {
	im := &customerImporter{opts: optsFor(uuid.New())}
	r := dataio.NewRow()
	r.Set(colEmail, "  ADA@Example.COM  ")
	func() {
		defer func() { _ = recover() }() // the ref is stamped before the nil service is reached
		_, _ = im.Row(context.Background(), r)
	}()
	if r.Ref != "ada@example.com" {
		t.Fatalf("ref = %q, want the lowered address so the operator can grep the file", r.Ref)
	}
}

// A bad phone is refused before the contact is written - the zero service has no repository, so reaching
// EnsureContact would panic - and a dry run reports it too
func TestImportRefusesABadPhoneBeforeWriting(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		opts := optsFor(uuid.New())
		opts.DryRun = dryRun
		im := &customerImporter{customers: &customer.Service{}, opts: opts}
		r := dataio.NewRow()
		r.Set(colEmail, "ada@example.com")
		r.Set(colPhone, "+381 64")
		if _, err := im.Row(context.Background(), r); !errors.Is(err, phone.ErrInvalid) {
			t.Errorf("dry run %v: row with a bad phone = %v, want phone.ErrInvalid", dryRun, err)
		}
	}
}

// The import-over-existing rule: a blank cell must never blank a profile field
func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("csv", "existing"); got != "csv" {
		t.Errorf("got %q, want the file value winning when present", got)
	}
	if got := firstNonEmpty("", "existing"); got != "existing" {
		t.Errorf("got %q, want the existing value kept for a blank cell", got)
	}
}

// Both files feed downstream spreadsheets keyed by column name - a rename breaks silently on their side
func TestColumnNamesStayAccountingStable(t *testing.T) {
	wantCustomer := []string{"email", "first_name", "last_name", "phone", "group", "is_active", "email_verified", "created_at"}
	if got := names(customerFields); strings.Join(got, ",") != strings.Join(wantCustomer, ",") {
		t.Errorf("customer columns = %v, want %v", got, wantCustomer)
	}
	have := map[string]bool{}
	for _, n := range names(orderFields) {
		have[n] = true
	}
	for _, must := range []string{"order_number", "sku", "quantity", "unit_price_cents", "line_total_cents", "order_total_cents", "currency"} {
		if !have[must] {
			t.Errorf("order columns are missing %q", must)
		}
	}
}

func names(fields []dataio.Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}

// Order lines are an export - an importable one would let a spreadsheet invent revenue
func TestResourcesRegister(t *testing.T) {
	reg := dataio.NewRegistry()
	reg.Register(CustomerResource(nil, nil))
	reg.Register(OrderLineResource(nil))

	customers, err := reg.Get("customers")
	if err != nil || !customers.CanImport() || !customers.CanExport() {
		t.Fatalf("customers should round-trip: %v", err)
	}
	orders, err := reg.Get("order_lines")
	if err != nil {
		t.Fatalf("get order_lines: %v", err)
	}
	if orders.CanImport() {
		t.Fatal("order lines must not be importable")
	}
}
