package customerio

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
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

// The import-over-existing rule: a blank cell must never blank a profile field
func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("csv", "existing"); got != "csv" {
		t.Errorf("got %q, want the file value winning when present", got)
	}
	if got := firstNonEmpty("", "existing"); got != "existing" {
		t.Errorf("got %q, want the existing value kept for a blank cell", got)
	}
}

// The file feeds downstream spreadsheets keyed by column name - a rename breaks silently on their side
func TestColumnNamesStayStable(t *testing.T) {
	want := []string{"email", "first_name", "last_name", "phone", "group", "is_active", "email_verified", "created_at"}
	if got := names(customerFields); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("customer columns = %v, want %v", got, want)
	}
}

func names(fields []dataio.Field) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Name
	}
	return out
}

func TestResourceRegisters(t *testing.T) {
	reg := dataio.NewRegistry()
	reg.Register(CustomerResource(nil, nil))
	customers, err := reg.Get("customers")
	if err != nil || !customers.CanImport() || !customers.CanExport() {
		t.Fatalf("customers should round-trip: %v", err)
	}
}
