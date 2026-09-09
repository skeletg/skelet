package e2e

import (
	"os"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/wiring"
)

// The GraphQL endpoints move when ADMIN_API_SLUG or FRONT_API_SLUG is set, and the suite runs against the same
// environment the server booted from - hardcoding the historical paths made every admin test 404 on an install
// that had turned the slug on, which is exactly the configuration most likely to be running

func adminAPIPath(t *testing.T) string {
	t.Helper()
	admin, _, err := wiring.APIPaths(os.Getenv)
	if err != nil {
		t.Fatalf("resolve admin api path: %v", err)
	}
	return admin
}

func frontAPIPath(t *testing.T) string {
	t.Helper()
	_, front, err := wiring.APIPaths(os.Getenv)
	if err != nil {
		t.Fatalf("resolve front api path: %v", err)
	}
	return front
}
