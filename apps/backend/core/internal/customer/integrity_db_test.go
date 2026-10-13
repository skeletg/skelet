package customer

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// Every write that reports itself does so in its own transaction - nothing on the bus beside the outbox, nothing for a write that failed
func TestAccountWritesEnqueueTheirEventsWithTheWrite(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	box := f.withOutbox()
	g := f.newGroup(t)

	c, err := f.svc.Register(f.ctx, ch, randomEmail(), "password123", "A", "B", nil)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.FlushMail()
	admin, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{ChannelID: ch, Email: randomEmail(), Password: "password123"})
	if err != nil {
		t.Fatal(err)
	}
	note := "vip"
	steps := []func() error{
		func() error {
			_, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{FirstName: "Ana"})
			return err
		},
		func() error {
			_, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Note: &note})
			return err
		},
		func() error { _, err := f.svc.SetCustomerActive(f.ctx, c.ID, false); return err },
		func() error { _, err := f.svc.SetCustomerGroup(f.ctx, c.ID, g.ID); return err },
		func() error { _, err := f.svc.SetTaxStatus(f.ctx, c.ID, true, "cert-1"); return err },
		func() error { _, err := f.svc.SetVatID(f.ctx, c.ID, "DE123", "valid"); return err },
		func() error { return f.svc.Delete(f.ctx, admin.ID) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{EventRegistered, EventRegistered, EventProfileUpdated, EventUpdated, EventUpdated, EventUpdated, EventUpdated, EventUpdated, EventDeleted}
	if got := box.types(); !slices.Equal(got, want) {
		t.Errorf("enqueued %v, want %v", got, want)
	}
	if _, err := f.svc.Register(f.ctx, ch, c.Email, "password123", "", "", nil); !errors.Is(err, ErrEmailTaken) {
		t.Fatal(err)
	}
	if _, err := f.svc.SetTaxStatus(f.ctx, uuid.New(), true, ""); err == nil {
		t.Fatal("tax status set on a customer that doesn't exist")
	}
	f.svc.FlushMail()
	if got := box.types(); len(got) != len(want) {
		t.Errorf("failed writes enqueued %v", got[len(want):])
	}
}

// The tax and VAT setters are admin edits like any other - the same veto, the same event, field names in camelCase
func TestTaxAndVatChangesGoThroughTheUpdateVeto(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	var vetoed []string
	f.svc.bus.SubscribeSync(EventUpdating, 100, func(_ context.Context, e events.Event) error {
		u := e.(*CustomerUpdating)
		vetoed = append(vetoed, u.ChangedFields...)
		if slices.Contains(u.ChangedFields, FieldVatID) {
			return errors.New("no vat changes today")
		}
		return nil
	})
	if _, err := f.svc.SetTaxStatus(f.ctx, c.ID, true, "cert-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetVatID(f.ctx, c.ID, "DE123", "valid"); err == nil {
		t.Error("a vetoed VAT change went through")
	}
	if got, _ := f.svc.GetByID(f.ctx, c.ID); got.VatID != "" || !got.TaxExempt {
		t.Errorf("vat %q exempt %v, want the veto to hold and the tax change to land", got.VatID, got.TaxExempt)
	}
	if !slices.Equal(vetoed, []string{FieldTaxExempt, FieldTaxExemptionNote, FieldVatID}) {
		t.Errorf("the veto saw %v", vetoed)
	}
}

func TestProfileUpdateNamesFieldsInCamelCase(t *testing.T) {
	f := newFixture(t)
	c := f.register(t, f.newChannel(t), randomEmail(), "password123")
	box := f.withOutbox()
	phone := "+1 555"
	if _, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{FirstName: "New", LastName: c.LastName, Phone: &phone}); err != nil {
		t.Fatal(err)
	}
	ev := box.events[0].(*CustomerProfileUpdated)
	if !slices.Equal(ev.ChangedFields, []string{FieldFirstName, FieldPhone}) {
		t.Errorf("changed fields %v, want only what changed, camelCase", ev.ChangedFields)
	}
}

// An admin edit that started while another transaction held the row must merge onto what that one committed
func TestConcurrentAdminEditsDoNotUndoEachOther(t *testing.T) {
	f := newFixture(t)
	c := f.register(t, f.newChannel(t), randomEmail(), "password123")
	tx, err := f.conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE customers SET last_name = 'Committed' WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	first := "Edited"
	go func() {
		_, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{FirstName: &first})
		done <- err
	}()
	waitForLockWait(t, f)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, _ := f.svc.GetByID(f.ctx, c.ID)
	if got.FirstName != "Edited" || got.LastName != "Committed" {
		t.Errorf("name = %q %q, want the edit merged onto the committed last name", got.FirstName, got.LastName)
	}
}

func waitForLockWait(t *testing.T, f *fixture) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := f.conn.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the second writer never queued behind the row lock")
}

func TestAddressesAreScopedToTheirOwner(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	ana, bob := f.register(t, ch, randomEmail(), "password123"), f.register(t, ch, randomEmail(), "password123")
	bobs := f.addAddress(t, bob.ID)
	var actions []string
	f.svc.bus.Subscribe(EventAddressChanged, func(_ context.Context, e events.Event) error {
		actions = append(actions, e.(*CustomerAddressChanged).Action)
		return nil
	})

	if _, err := f.svc.UpdateAddress(f.ctx, ana.ID, bobs.ID, addressInput()); !errors.Is(err, ErrAddressNotFound) {
		t.Errorf("update of another customer's address = %v, want ErrAddressNotFound", err)
	}
	if err := f.svc.DeleteAddress(f.ctx, ana.ID, bobs.ID); !errors.Is(err, ErrAddressNotFound) {
		t.Errorf("delete of another customer's address = %v, want ErrAddressNotFound", err)
	}
	if _, err := f.svc.SetDefaultBillingAddress(f.ctx, ana.ID, bobs.ID); !errors.Is(err, ErrAddressNotFound) {
		t.Errorf("default billing to another customer's address = %v, want ErrAddressNotFound", err)
	}
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, ana.ID, AdminUpdateCustomerInput{DefaultShippingAddressID: &bobs.ID}); !errors.Is(err, ErrAddressNotFound) {
		t.Errorf("admin default to another customer's address = %v, want ErrAddressNotFound", err)
	}
	if _, err := f.svc.SetDefaultShippingAddress(f.ctx, bob.ID, bobs.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.AdminDeleteAddress(f.ctx, bobs.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.bus.WaitIdle(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(actions, []string{AddressDefaultShipping, AddressDeleted}) {
		t.Errorf("address events %v, want the default and the admin delete announced", actions)
	}
}

func TestAddressCountryIsNormalizedAndChecked(t *testing.T) {
	f := newFixture(t)
	c := f.register(t, f.newChannel(t), randomEmail(), "password123")
	in := addressInput()
	in.CountryCode = " us "
	a, err := f.svc.AddAddress(f.ctx, c.ID, in)
	if err != nil || a.CountryCode != "US" {
		t.Fatalf("lower-case country = %q (%v), want stored as US", a.CountryCode, err)
	}
	for _, code := range []string{"QQ", "usa"} {
		in.CountryCode = code
		if _, err := f.svc.AddAddress(f.ctx, c.ID, in); !errors.Is(err, ErrUnknownCountry) {
			t.Errorf("country %q = %v, want ErrUnknownCountry", code, err)
		}
	}
}

// The order count is a pre-check - an order that lands after it still has to be refused as one, not as a server fault
func TestDeleteRacingAnOrderReadsAsHasOrders(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	f.seedOrder(t, ch, c.ID)
	err := f.svc.atomically(f.ctx, func(r *postgresRepository) ([]events.Event, error) { return nil, r.Delete(f.ctx, c) })
	if !errors.Is(err, ErrCustomerHasOrders) {
		t.Errorf("delete with an order = %v, want ErrCustomerHasOrders", err)
	}
}

func TestGroupRacesAndMissesAreSentinels(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	g := f.newGroup(t)
	c := f.register(t, ch, randomEmail(), "password123")
	if _, err := f.svc.GetGroup(f.ctx, uuid.New()); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("unknown group = %v, want ErrGroupNotFound", err)
	}
	if _, err := f.svc.SetCustomerGroup(f.ctx, c.ID, uuid.New()); !errors.Is(err, ErrGroupNotFound) {
		t.Errorf("unknown group assignment = %v, want ErrGroupNotFound", err)
	}
	if _, err := f.svc.SetCustomerGroup(f.ctx, c.ID, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.repo.queries.DeleteCustomerGroup(f.ctx, g.ID); err == nil {
		t.Fatal("the statement deleted a group with members")
	}
	// The member count normally refuses first; with it out of the way the FK has to read the same
	if err := deleteGroupRow(f.ctx, f.svc.repo, g.ID); !errors.Is(err, ErrGroupInUse) {
		t.Errorf("delete racing a member = %v, want ErrGroupInUse", err)
	}
}

// A swap that loses to one committed first reports the conflict instead of a raw unique violation
func TestDefaultGroupSwapRaceIsAConflict(t *testing.T) {
	f := newFixture(t)
	original, err := f.svc.GetDefaultGroup(f.ctx)
	if err != nil {
		t.Skipf("no default group: %v", err)
	}
	t.Cleanup(func() {
		_, _ = f.conn.Exec(`UPDATE customer_groups SET is_default = (id = $1)`, original.ID)
	})
	winner, loser := f.newGroup(t), f.newGroup(t)
	tx, err := f.conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE customer_groups SET is_default = FALSE WHERE is_default`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE customer_groups SET is_default = TRUE WHERE id = $1`, winner.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := f.svc.SetDefaultGroup(f.ctx, loser.ID); done <- err }()
	waitForLockWait(t, f)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrDefaultGroupConflict) {
		t.Errorf("losing swap = %v, want ErrDefaultGroupConflict", err)
	}
}

func TestListPageScopeAndSearchAreLiteral(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	tag := uuid.NewString()[:6]
	pct, err := f.svc.Register(f.ctx, ch, randomEmail(), "password123", "P"+tag+"%x", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Register(f.ctx, ch, randomEmail(), "password123", "P"+tag+"Yx", "", nil); err != nil {
		t.Fatal(err)
	}
	f.svc.FlushMail()

	if got, n, err := f.svc.ListPage(f.ctx, ListPageFilter{ChannelIDs: []uuid.UUID{}}); err != nil || len(got) != 0 || n != 0 {
		t.Errorf("an empty channel scope listed %d (%v), want nothing", len(got), err)
	}
	got, _, err := f.svc.ListPage(f.ctx, ListPageFilter{ChannelIDs: []uuid.UUID{ch}, Search: "P" + tag + "%"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != pct.ID {
		t.Errorf("search for a literal %% matched %d customers, want only the one whose name has it", len(got))
	}
	if got[0].Group == nil || got[0].Group.Slug == "" {
		t.Error("the listed customer came back without its group")
	}
	all, _, err := f.svc.ListPage(f.ctx, ListPageFilter{Search: "P" + tag})
	if err != nil || len(all) != 2 {
		t.Errorf("unscoped search found %d (%v), want both", len(all), err)
	}
	if strings.Contains(escapeLike(`a\b`), `a\b`) {
		t.Error("a backslash in the search was left to escape the next character")
	}
}
