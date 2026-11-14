package auth

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestADownloadTicketOpensOnlyItsOwnPathBriefly(t *testing.T) {
	keys := testAdminKeys(t)
	admin := &Claims{AdminID: uuid.New(), SessionID: uuid.New(), Email: "ada@example.com"}
	file := "/api/admin/media/file/" + uuid.NewString()
	ticket, err := GenerateDownloadTicket(admin, file, keys, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	expired, _ := GenerateDownloadTicket(admin, file, keys, -time.Second)
	access, _ := GenerateToken(admin.AdminID, admin.SessionID, admin.Email, false, keys, time.Minute)
	other, _ := NewAdminKeys("a-different-admin-secret-0123456789")
	forged, _ := GenerateDownloadTicket(admin, file, other, time.Minute)

	seen := func(method, target string, header http.Header) *Claims {
		var got *Claims
		h := DownloadTicket(keys)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got, _ = AdminFromContext(r.Context()) }))
		req := httptest.NewRequest(method, target, nil)
		maps.Copy(req.Header, header)
		AdminMiddleware(keys)(h).ServeHTTP(httptest.NewRecorder(), req)
		return got
	}

	if got := seen(http.MethodGet, file+"?ticket="+ticket, nil); got == nil || got.AdminID != admin.AdminID || got.SessionID != admin.SessionID {
		t.Fatalf("its own path: %+v, want the admin on the session it was minted on", got)
	}
	for name, tc := range map[string]struct{ method, target string }{
		"another file":          {http.MethodGet, "/api/admin/media/file/" + uuid.NewString() + "?ticket=" + ticket},
		"a write":               {http.MethodPost, file + "?ticket=" + ticket},
		"once it has run out":   {http.MethodGet, file + "?ticket=" + expired},
		"signed by another key": {http.MethodGet, file + "?ticket=" + forged},
		"an access token":       {http.MethodGet, file + "?ticket=" + access},
	} {
		if got := seen(tc.method, tc.target, nil); got != nil {
			t.Errorf("%s: signed in as %s", name, got.AdminID)
		}
	}
	// a ticket is no bearer, and the admin surface never takes one as such
	if _, err := ValidateToken(ticket, keys); err == nil {
		t.Error("a download ticket was taken for an access token")
	}
	bearer := &Claims{AdminID: uuid.New()}
	signedIn, _ := GenerateToken(bearer.AdminID, uuid.Nil, "", false, keys, time.Minute)
	if got := seen(http.MethodGet, file+"?ticket="+ticket, http.Header{"Authorization": {"Bearer " + signedIn}}); got == nil || got.AdminID != bearer.AdminID {
		t.Errorf("a request with a bearer was signed in as %+v, want the bearer's admin", got)
	}
}
