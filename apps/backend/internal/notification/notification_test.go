package notification

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func notif(perm string, channelID *uuid.UUID) Notification {
	return Notification{ID: uuid.New(), RequiredPermission: perm, ChannelID: channelID, Title: "t"}
}

func categorised(category, perm string) Notification {
	n := notif(perm, nil)
	n.Category = category
	return n
}

// A mute is a preference, not a permission, so it outranks the bypass a superuser rides on - and the same predicate is
// in the list, count and mark-all SQL, or the badge would count what the inbox refuses to show
func TestCanSeeHonoursMutedCategories(t *testing.T) {
	sc := Scope{AdminID: uuid.New(), Bypass: true, Muted: []string{CategoryReview}}

	if sc.CanSee(categorised(CategoryReview, PermReviews)) {
		t.Error("a muted category was visible to a bypass admin")
	}
	if !sc.CanSee(categorised(CategoryReturn, PermOrders)) {
		t.Error("an unmuted category was refused")
	}

	// a category nobody produces is harmless - the pref stores free text, so it just matches nothing
	junk := Scope{AdminID: uuid.New(), Bypass: true, Muted: []string{"not-a-category"}}
	if !junk.CanSee(categorised(CategorySystem, PermSystem)) {
		t.Error("an unknown muted value hid a real notification")
	}

	// muting does not widen anything either - the permission still has to be held
	scoped := Scope{AdminID: uuid.New(), Perms: []string{PermOrders}, Muted: []string{CategoryReview}}
	if scoped.CanSee(categorised(CategoryInventory, PermInventory)) {
		t.Error("muting one category let another through without its permission")
	}
}

// CanSee is the in-memory mirror of the List SQL, and it's the only gate on the live subscription
// push - a notification that slips through here reaches an admin the inbox query would never show
func TestCanSeeRequiresThePermission(t *testing.T) {
	sc := Scope{AdminID: uuid.New(), Perms: []string{PermOrders, PermReviews}}

	if !sc.CanSee(notif(PermOrders, nil)) {
		t.Error("a held permission was refused")
	}
	if !sc.CanSee(notif(PermReviews, nil)) {
		t.Error("a held permission was refused")
	}
	for _, perm := range []string{PermInventory, PermSystem, PermForms} {
		if sc.CanSee(notif(perm, nil)) {
			t.Errorf("%q was visible without the permission", perm)
		}
	}
}

// A producer that forgets to stamp a permission must fail closed - an empty required permission is
// not "no permission needed"
func TestCanSeeRefusesAnUnstampedNotification(t *testing.T) {
	sc := Scope{AdminID: uuid.New(), Perms: []string{PermOrders}}
	if sc.CanSee(notif("", nil)) {
		t.Error("a notification with no required permission was visible, want it refused")
	}
	// an admin holding no permissions at all sees nothing
	none := Scope{AdminID: uuid.New()}
	if none.CanSee(notif(PermOrders, nil)) {
		t.Error("an admin with no permissions saw an orders notification")
	}
}

// Channel scoping is the second half of the filter - holding orders.view on one channel must not
// surface another channel's orders
func TestCanSeeScopesToPermittedChannels(t *testing.T) {
	mine, theirs := uuid.New(), uuid.New()
	sc := Scope{AdminID: uuid.New(), Perms: []string{PermOrders}, Channels: []uuid.UUID{mine}}

	if !sc.CanSee(notif(PermOrders, &mine)) {
		t.Error("a notification on a permitted channel was refused")
	}
	if sc.CanSee(notif(PermOrders, &theirs)) {
		t.Error("a notification on another channel was visible")
	}
	if !sc.CanSee(notif(PermOrders, nil)) {
		t.Error("a global notification was refused, want it visible to anyone holding the permission")
	}

	// an admin with the permission but no channels still sees global notices only
	noChannels := Scope{AdminID: uuid.New(), Perms: []string{PermOrders}}
	if noChannels.CanSee(notif(PermOrders, &mine)) {
		t.Error("an admin with no permitted channels saw a channel-scoped notification")
	}
	if !noChannels.CanSee(notif(PermOrders, nil)) {
		t.Error("an admin with no permitted channels was refused a global notification")
	}
}

// Superusers and all-channel admins short-circuit both filters, including the unstamped case
func TestCanSeeBypassSeesEverything(t *testing.T) {
	sc := Scope{AdminID: uuid.New(), Bypass: true}
	other := uuid.New()
	for name, n := range map[string]Notification{
		"unstamped":       notif("", nil),
		"foreign channel": notif(PermInventory, &other),
		"global":          notif(PermSystem, nil),
	} {
		if !sc.CanSee(n) {
			t.Errorf("bypass scope refused the %s notification", name)
		}
	}
}

// The permission list is the bounded set producers may stamp - one that isn't in it can never be
// granted, so the notification would be invisible to every non-superuser
func TestEveryCategoryPermissionIsGrantable(t *testing.T) {
	if len(CategoryPermissions) == 0 {
		t.Fatal("CategoryPermissions is empty")
	}
	seen := make(map[string]bool, len(CategoryPermissions))
	for _, perm := range CategoryPermissions {
		if perm == "" {
			t.Error("CategoryPermissions contains an empty slug")
		}
		if seen[perm] {
			t.Errorf("%q is listed twice", perm)
		}
		seen[perm] = true

		sc := Scope{AdminID: uuid.New(), Perms: []string{perm}}
		if !sc.CanSee(notif(perm, nil)) {
			t.Errorf("an admin holding %q could not see a notification stamped with it", perm)
		}
	}
	for _, perm := range []string{PermOrders, PermReviews, PermInventory, PermSystem, PermForms} {
		if !seen[perm] {
			t.Errorf("%q is a declared permission but is missing from CategoryPermissions", perm)
		}
	}
}

// Every listener gets its own copy - a second dashboard tab must not steal the first one's push
func TestHubFansOutToEverySubscriber(t *testing.T) {
	h := NewHub(nil) // no Valkey wired, so the hub fans out in-process
	a, cancelA := h.Subscribe()
	b, cancelB := h.Subscribe()
	defer cancelA()
	defer cancelB()

	sent := notif(PermOrders, nil)
	h.fanOut(sent)

	for name, ch := range map[string]<-chan Notification{"a": a, "b": b} {
		select {
		case got := <-ch:
			if got.ID != sent.ID {
				t.Errorf("listener %s got %v, want %v", name, got.ID, sent.ID)
			}
		case <-time.After(time.Second):
			t.Errorf("listener %s received nothing", name)
		}
	}
}

// The cancel func has to actually unregister, or a closed subscription keeps a channel alive and
// fills up forever
func TestHubCancelStopsDelivery(t *testing.T) {
	h := NewHub(nil)
	ch, cancel := h.Subscribe()
	cancel()

	h.fanOut(notif(PermOrders, nil))
	select {
	case got := <-ch:
		t.Errorf("a cancelled subscriber received %v, want nothing", got.ID)
	case <-time.After(50 * time.Millisecond):
	}

	h.mu.Lock()
	n := len(h.subs)
	h.mu.Unlock()
	if n != 0 {
		t.Errorf("%d subscribers still registered after cancel, want 0", n)
	}
}

// Delivery is deliberately at-most-once - a listener that stops reading must be dropped rather than
// block the producer, since the inbox poll backfills whatever it missed
func TestHubDropsForASlowListenerInsteadOfBlocking(t *testing.T) {
	h := NewHub(nil)
	ch, cancel := h.Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 200 { // well past the 16-deep buffer
			h.fanOut(notif(PermOrders, nil))
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fanOut blocked on a listener that never read, want the message dropped")
	}
	if len(ch) == 0 {
		t.Error("the listener buffered nothing at all, want it filled to capacity")
	}
}

// Publish with no Valkey client wired has to still reach local listeners, which is what the CLI and
// tests run on
func TestPublishWithoutValkeyStillFansOutLocally(t *testing.T) {
	h := NewHub(nil)
	ch, cancel := h.Subscribe()
	defer cancel()

	sent := notif(PermReviews, nil)
	h.Publish(t.Context(), sent)

	select {
	case got := <-ch:
		if got.ID != sent.ID {
			t.Errorf("got %v, want %v", got.ID, sent.ID)
		}
	case <-time.After(time.Second):
		t.Error("no local delivery without a Valkey client")
	}
}

// A nil hub is the "notifications disabled" wiring, and every call site treats it as a no-op
func TestNilHubIsANoOp(t *testing.T) {
	var h *Hub
	h.Publish(t.Context(), notif(PermOrders, nil)) // must not panic
	h.Run(t.Context())
}

// Subscribe and fanOut race across every connected dashboard - run under -race
func TestHubIsSafeUnderConcurrentSubscribeAndFanOut(t *testing.T) {
	h := NewHub(nil)
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				h.fanOut(notif(PermOrders, nil))
			}
		}
	})

	wg.Go(func() {
		for range 500 {
			_, cancel := h.Subscribe()
			cancel()
		}
	})

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}
