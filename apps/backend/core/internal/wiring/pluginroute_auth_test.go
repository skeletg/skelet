package wiring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestPluginRoutesSeeTheShopperButNotAnAdmin(t *testing.T) {
	const secret = "plugin-route-secret"
	active := func(context.Context, uuid.UUID) (bool, error) { return true, nil }
	var customer, admin bool
	h := pluginRouteAuth(secret, active)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, customer = authpkg.CustomerFromContext(r.Context())
		_, admin = authpkg.AdminFromContext(r.Context())
	}))
	call := func(tok string) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/x/hook", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		h.ServeHTTP(httptest.NewRecorder(), req)
	}

	ctok, err := authpkg.GenerateCustomerToken(uuid.New(), uuid.New(), "s@example.com", secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	call(ctok)
	if !customer {
		t.Error("a signed-in shopper reached the route anonymous")
	}
	atok, err := authpkg.GenerateToken(uuid.New(), uuid.New(), "a@example.com", true, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	call(atok)
	if admin {
		t.Error("an admin token reached a plugin route as an admin")
	}
}
