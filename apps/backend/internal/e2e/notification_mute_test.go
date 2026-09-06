package e2e

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/notification"
)

// Scenario: an admin mutes a notification category, over the real API
//
// The point of the test is that the mute reaches ALL of the read paths at once. The badge is
// counted in SQL, the list is a different query and mark-all is a third - a mute honoured by some
// of them is worse than no mute at all, because the badge would then count rows the inbox refuses
// to show. The admin here is a superuser, so it also pins that a preference outranks scope bypass
func TestNotificationMuteAppliesToListCountAndMarkAll(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	email := fmt.Sprintf("notif-mute-%d@example.com", time.Now().UnixNano())
	if err := h.D.Admin.CreateSuperuser(h.Ctx, email, "notif-mute-password-1", "Mute", "Tester"); err != nil {
		t.Fatalf("create superuser: %v", err)
	}
	login, errs := h.adminGqlAuthed(t, ts, "", `mutation($e: String!, $p: String!) {
		adminLogin(input: {email: $e, password: $p}) { accessToken }
	}`, map[string]any{"e": email, "p": "notif-mute-password-1"})
	if len(errs) > 0 {
		t.Fatalf("adminLogin: %v", errs)
	}
	bearer, _ := login["adminLogin"].(map[string]any)["accessToken"].(string)
	if bearer == "" {
		t.Fatal("adminLogin returned no access token")
	}

	stamp := time.Now().UnixNano()
	post := func(category, perm, title string) {
		t.Helper()
		if err := h.D.Notification.Create(h.Ctx, notification.Input{
			Category:           category,
			Severity:           notification.SeverityWarning,
			Title:              title,
			Body:               "b",
			RequiredPermission: perm,
			EntityType:         "test",
			EntityID:           "1",
			DedupeKey:          fmt.Sprintf("mute-e2e:%s:%d", category, stamp),
		}); err != nil {
			t.Fatalf("create %s notification: %v", category, err)
		}
	}
	post(notification.CategoryReview, notification.PermReviews, "a review needs moderation")
	post(notification.CategoryReturn, notification.PermOrders, "a return was requested")

	// The suite shares one disposable database, so other scenarios' notifications are in here too -
	// every assertion below is a delta against what this admin can see right now
	type row struct {
		title    string
		category string
		read     bool
	}
	list := func() []row {
		t.Helper()
		data, errs := h.adminGqlAuthed(t, ts, bearer, `query { notifications(limit: 200) { title category read } }`, nil)
		if len(errs) > 0 {
			t.Fatalf("notifications: %v", errs)
		}
		raw, _ := data["notifications"].([]any)
		out := make([]row, 0, len(raw))
		for _, r := range raw {
			m := r.(map[string]any)
			read, _ := m["read"].(bool)
			out = append(out, row{title: m["title"].(string), category: m["category"].(string), read: read})
		}
		return out
	}
	unread := func() float64 {
		t.Helper()
		data, errs := h.adminGqlAuthed(t, ts, bearer, `query { notificationUnreadCount }`, nil)
		if len(errs) > 0 {
			t.Fatalf("notificationUnreadCount: %v", errs)
		}
		c, _ := data["notificationUnreadCount"].(float64)
		return c
	}
	has := func(rows []row, want string) bool {
		for _, r := range rows {
			if r.title == want {
				return true
			}
		}
		return false
	}
	countCategory := func(rows []row, category string, unreadOnly bool) int {
		n := 0
		for _, r := range rows {
			if r.category == category && (!unreadOnly || !r.read) {
				n++
			}
		}
		return n
	}

	before, beforeCount := list(), unread()
	if !has(before, "a review needs moderation") || !has(before, "a return was requested") {
		t.Fatalf("both notifications should be visible before muting, got %d rows", len(before))
	}
	// the count query has no limit while the list does, so the arithmetic below only holds untruncated
	exact := len(before) < 200
	mutedUnread := countCategory(before, notification.CategoryReview, true)

	if _, errs := h.adminGqlAuthed(t, ts, bearer,
		`mutation($k: String!, $v: JSON) { setMyPreference(key: $k, value: $v) }`,
		map[string]any{"k": notification.MutedCategoriesPref, "v": []string{notification.CategoryReview}},
	); len(errs) > 0 {
		t.Fatalf("setMyPreference: %v", errs)
	}

	after, afterCount := list(), unread()
	if n := countCategory(after, notification.CategoryReview, false); n != 0 {
		t.Errorf("%d rows of a muted category are still listed", n)
	}
	if !has(after, "a return was requested") {
		t.Errorf("muting one category hid another")
	}
	if exact && afterCount != beforeCount-float64(mutedUnread) {
		t.Errorf("unread count = %v, want %v - the badge has to drop with the list", afterCount, beforeCount-float64(mutedUnread))
	}

	// mark-all must skip the muted rows too, or unmuting later would silently swallow them
	if _, errs := h.adminGqlAuthed(t, ts, bearer, `mutation { markAllNotificationsRead }`, nil); len(errs) > 0 {
		t.Fatalf("markAllNotificationsRead: %v", errs)
	}
	if c := unread(); c != 0 {
		t.Errorf("unread after mark-all = %v, want 0", c)
	}
	if _, errs := h.adminGqlAuthed(t, ts, bearer,
		`mutation($k: String!, $v: JSON) { setMyPreference(key: $k, value: $v) }`,
		map[string]any{"k": notification.MutedCategoriesPref, "v": nil},
	); len(errs) > 0 {
		t.Fatalf("unmute: %v", errs)
	}
	if c := unread(); exact && c != float64(mutedUnread) {
		t.Errorf("unread after unmuting = %v, want the %d rows mark-all deliberately left alone", c, mutedUnread)
	}
	if rows := list(); !has(rows, "a review needs moderation") {
		t.Error("unmuting did not bring the category back")
	}
}
