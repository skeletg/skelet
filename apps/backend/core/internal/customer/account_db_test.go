package customer

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	phonepkg "github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

// Address ids come off the client, so ownership is all that stands between a customer and someone else's
func TestAddressWritesAreOwnerScoped(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	owner := f.register(t, ch, randomEmail(), "password123")
	stranger := f.register(t, ch, randomEmail(), "password123")
	addr := f.addAddress(t, owner.ID)

	for name, call := range map[string]func() error{
		"update": func() error {
			_, err := f.svc.UpdateAddress(f.ctx, stranger.ID, addr.ID, addressInput())
			return err
		},
		"delete": func() error { return f.svc.DeleteAddress(f.ctx, stranger.ID, addr.ID) },
		"set default billing": func() error {
			_, err := f.svc.SetDefaultBillingAddress(f.ctx, stranger.ID, addr.ID)
			return err
		},
		"set default shipping": func() error {
			_, err := f.svc.SetDefaultShippingAddress(f.ctx, stranger.ID, addr.ID)
			return err
		},
	} {
		if err := call(); err == nil {
			t.Errorf("%s by a non-owner was allowed", name)
		}
	}
	got, err := f.svc.GetAddressByID(f.ctx, addr.ID)
	if err != nil || got.CustomerID != owner.ID {
		t.Errorf("address after the refused writes = %v / %v, want still owned by %s", got.CustomerID, err, owner.ID)
	}
}

// Empty means "not supplied", so a phone-only edit must not wipe the name the customer never touched
func TestUpdateProfileMergesRatherThanBlanks(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	phone := "+12015550123"

	bad := "+1 555 000"
	if _, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &bad}); !errors.Is(err, phonepkg.ErrInvalid) {
		t.Fatalf("update with an invalid phone = %v, want phone.ErrInvalid", err)
	}
	spaced := "+1 (201) 555-0123"
	got, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &spaced})
	if err != nil {
		t.Fatalf("update phone: %v", err)
	}
	if got.FirstName != c.FirstName || got.LastName != c.LastName {
		t.Errorf("name after a phone-only edit = %q %q, want %q %q", got.FirstName, got.LastName, c.FirstName, c.LastName)
	}
	if got.Phone == nil || *got.Phone != phone {
		t.Fatalf("phone = %v, want %q", got.Phone, phone)
	}

	got, err = f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{FirstName: "Renamed"})
	if err != nil {
		t.Fatalf("update first name: %v", err)
	}
	if got.FirstName != "Renamed" || got.LastName != c.LastName {
		t.Errorf("name after a first-name edit = %q %q, want \"Renamed\" %q", got.FirstName, got.LastName, c.LastName)
	}
	if got.Phone == nil || *got.Phone != phone {
		t.Errorf("phone = %v after a name-only edit, want %q kept", got.Phone, phone)
	}
}

// Admins paste addresses out of support tickets, and an unnormalized one can never match at login
func TestAdminUpdateNormalizesTheEmailAndLeavesNilFieldsAlone(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	f.markVerified(t, c.ID)
	pasted := "  Rebranded-" + uuid.NewString()[:8] + "@Example.INVALID "

	got, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Email: &pasted})
	if err != nil {
		t.Fatalf("admin update: %v", err)
	}
	if want := NormalizeEmail(pasted); got.Email != want {
		t.Errorf("stored email = %q, want %q", got.Email, want)
	}
	if got.FirstName != c.FirstName || got.LastName != c.LastName || got.IsActive != c.IsActive {
		t.Error("nil input fields overwrote the existing row")
	}
	// re-verification is an explicit isEmailVerified:false in the same call, never a side effect
	if !got.IsEmailVerified {
		t.Error("changing the address silently cleared the verified flag")
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, strings.TrimSpace(pasted), "password123"); err != nil {
		t.Errorf("login under the new address: %v", err)
	}
}

// Metadata replaces wholesale rather than merging, and nil has to mean "leave it"
func TestAdminUpdateMetadataKeepsOnNilAndReplacesOnValue(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")

	first := Metadata{"tier": "gold", "source": "import"}
	got, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Metadata: &first})
	if err != nil {
		t.Fatalf("set metadata: %v", err)
	}
	if got.Metadata["tier"] != "gold" || got.Metadata["source"] != "import" {
		t.Fatalf("metadata = %v, want both entries", got.Metadata)
	}
	name := "Kept"
	if got, err = f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{FirstName: &name}); err != nil {
		t.Fatalf("unrelated update: %v", err)
	}
	if got.Metadata["tier"] != "gold" {
		t.Errorf("metadata = %v after an unrelated edit, want it untouched", got.Metadata)
	}
	replacement := Metadata{"tier": "silver"}
	if got, err = f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Metadata: &replacement}); err != nil {
		t.Fatalf("replace metadata: %v", err)
	}
	if got.Metadata["tier"] != "silver" || len(got.Metadata) != 1 {
		t.Errorf("metadata = %v, want the map replaced wholesale", got.Metadata)
	}
}

// A default address id comes straight off the admin form, so it has to be checked against the owner
func TestAdminUpdateRefusesAnotherCustomersAddress(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	owner := f.register(t, ch, randomEmail(), "password123")
	stranger := f.register(t, ch, randomEmail(), "password123")
	addr := f.addAddress(t, owner.ID)

	if _, err := f.svc.AdminUpdateCustomer(f.ctx, stranger.ID, AdminUpdateCustomerInput{DefaultBillingAddressID: &addr.ID}); err == nil {
		t.Error("another customer's address was accepted as the default billing address")
	}
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, stranger.ID, AdminUpdateCustomerInput{DefaultShippingAddressID: &addr.ID}); err == nil {
		t.Error("another customer's address was accepted as the default shipping address")
	}
	got, err := f.svc.GetByID(f.ctx, stranger.ID)
	if err != nil {
		t.Fatalf("re-read customer: %v", err)
	}
	if got.DefaultBillingAddressID != nil || got.DefaultShippingAddressID != nil {
		t.Error("a refused write still set a default address")
	}
}

// An admin-created account is vouched for, so it lands active and verified without an emailed link
func TestAdminCreateDefaultsToAnUsableAccount(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := "  Admin-" + uuid.NewString()[:8] + "@Example.INVALID "

	c, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{
		ChannelID: ch, Email: addr, Password: "password123", FirstName: "Admin", LastName: "Made",
	})
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}
	if !c.IsActive || !c.IsEmailVerified {
		t.Errorf("created active=%v verified=%v, want both true", c.IsActive, c.IsEmailVerified)
	}
	if want := NormalizeEmail(addr); c.Email != want {
		t.Errorf("stored email = %q, want %q", c.Email, want)
	}
	if c.Group == nil || !c.Group.IsDefault {
		t.Errorf("group = %v, want the default group for an empty slug", c.Group)
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, strings.TrimSpace(addr), "password123"); err != nil {
		t.Errorf("login with the admin-set password: %v", err)
	}
	if _, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{
		ChannelID: ch, Email: strings.ToUpper(addr), Password: "password123",
	}); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("duplicate admin create = %v, want ErrEmailTaken", err)
	}
}

// Group slugs feed promotion rules, so a group can't vanish out from under its members or the fallback
func TestCustomerGroupDeleteGuards(t *testing.T) {
	f := newFixture(t)
	// the group is created first so its cleanup runs last - the channel's customers still point at it
	group := f.newGroup(t)
	ch := f.newChannel(t)

	def, err := f.svc.GetDefaultGroup(f.ctx)
	if err != nil {
		t.Fatalf("default group: %v", err)
	}
	if err := f.svc.DeleteGroup(f.ctx, def.ID); !errors.Is(err, ErrCannotDeleteDefaultGroup) {
		t.Errorf("delete of the default group = %v, want ErrCannotDeleteDefaultGroup", err)
	}
	if _, err := f.svc.CreateGroup(f.ctx, CreateCustomerGroupInput{Name: "dupe", Slug: group.Slug}); !errors.Is(err, ErrGroupSlugTaken) {
		t.Errorf("duplicate slug = %v, want ErrGroupSlugTaken", err)
	}

	c := f.register(t, ch, randomEmail(), "password123")
	if _, err := f.svc.SetCustomerGroup(f.ctx, c.ID, group.ID); err != nil {
		t.Fatalf("assign group: %v", err)
	}
	if err := f.svc.DeleteGroup(f.ctx, group.ID); !errors.Is(err, ErrGroupInUse) {
		t.Fatalf("delete of a group with members = %v, want ErrGroupInUse", err)
	}
	if _, err := f.svc.SetCustomerGroup(f.ctx, c.ID, def.ID); err != nil {
		t.Fatalf("reassign to default: %v", err)
	}
	if err := f.svc.DeleteGroup(f.ctx, group.ID); err != nil {
		t.Errorf("delete of an empty non-default group: %v", err)
	}
}

// Promoting a group clears the old default in the same transaction, so there is never a window with
// zero defaults - registration resolves its group through that flag
func TestSetDefaultGroupLeavesExactlyOneDefault(t *testing.T) {
	f := newFixture(t)
	group := f.newGroup(t)
	original, err := f.svc.GetDefaultGroup(f.ctx)
	if err != nil {
		t.Fatalf("default group: %v", err)
	}
	t.Cleanup(func() { _, _ = f.svc.SetDefaultGroup(f.ctx, original.ID) })

	if _, err := f.svc.SetDefaultGroup(f.ctx, group.ID); err != nil {
		t.Fatalf("promote group: %v", err)
	}
	if got, err := f.svc.GetDefaultGroup(f.ctx); err != nil || got.ID != group.ID {
		t.Errorf("default group = %v / %v, want %s", got.ID, err, group.ID)
	}
	var defaults int
	if err := f.conn.QueryRow(`SELECT count(*) FROM customer_groups WHERE is_default`).Scan(&defaults); err != nil {
		t.Fatalf("count defaults: %v", err)
	}
	if defaults != 1 {
		t.Errorf("%d groups are marked default, want exactly 1", defaults)
	}
	if _, err := f.svc.SetDefaultGroup(f.ctx, group.ID); err != nil {
		t.Errorf("re-promoting the current default should be a no-op: %v", err)
	}
}
