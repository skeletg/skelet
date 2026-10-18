package e2e

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

// Scenario: the login mutation reports a genuinely first login, over the real API
func TestAdminLoginReportsTheFirstLoginOnce(t *testing.T) {
	h := newHarness(t)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()

	email := fmt.Sprintf("first-login-%d@example.com", time.Now().UnixNano())
	const password = "first-login-password-1"
	if _, err := h.D.Admin.CreateAdmin(h.Ctx, email, password, "First", "Login", nil); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	login := func() (firstLogin bool, lastLoginAt any, bearer string) {
		t.Helper()
		data, errs := h.adminGqlAuthed(t, ts, "", `mutation($e: String!, $p: String!) {
			adminLogin(input: {email: $e, password: $p}) {
				accessToken
				firstLogin
				admin { lastLoginAt }
			}
		}`, map[string]any{"e": email, "p": password})
		if len(errs) > 0 {
			t.Fatalf("adminLogin: %v", errs)
		}
		payload, _ := data["adminLogin"].(map[string]any)
		first, _ := payload["firstLogin"].(bool)
		token, _ := payload["accessToken"].(string)
		admin, _ := payload["admin"].(map[string]any)
		return first, admin["lastLoginAt"], token
	}

	first, stamped, bearer := login()
	if !first {
		t.Error("firstLogin = false on the very first login")
	}
	if bearer == "" {
		t.Fatal("no access token")
	}
	// the column is stamped by the same statement that reported the flag, so the payload already
	// carries a "last seen" for the admins list
	if stamped == nil {
		t.Error("lastLoginAt is still null after signing in")
	}

	if again, _, _ := login(); again {
		t.Error("firstLogin = true on the second login - the introduction would reappear forever")
	}

	// a refresh is not a login, and must never look like one
	data, errs := h.adminGqlAuthed(t, ts, "", `mutation($e: String!, $p: String!) {
		adminLogin(input: {email: $e, password: $p}) { refreshToken }
	}`, map[string]any{"e": email, "p": password})
	if len(errs) > 0 {
		t.Fatalf("adminLogin for refresh: %v", errs)
	}
	refresh, _ := data["adminLogin"].(map[string]any)["refreshToken"].(string)
	data, errs = h.adminGqlAuthed(t, ts, "", `mutation($t: String!) {
		refreshToken(token: $t) { firstLogin }
	}`, map[string]any{"t": refresh})
	if len(errs) > 0 {
		t.Fatalf("refreshToken: %v", errs)
	}
	if refreshed, _ := data["refreshToken"].(map[string]any)["firstLogin"].(bool); refreshed {
		t.Error("refreshToken reported firstLogin = true")
	}
}
