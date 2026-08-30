package plugin

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/sdk"
)

func disableAll(t *testing.T) {
	t.Helper()
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(context.Context, string, uuid.UUID) bool { return false }
}

// The scheduler holds the handler for the process's life, so a registration-time check would leave
// a disabled plugin's cron firing until the next deploy
func TestDisabledPluginJobDoesNotRun(t *testing.T) {
	disableAll(t)
	ran := false
	gated := gatedJobHandler("off", func(context.Context) error { ran = true; return nil })
	if err := gated(context.Background()); err != nil {
		t.Errorf("a skipped run should report success, got %v", err)
	}
	if ran {
		t.Error("disabled plugin's scheduled job still ran")
	}
}

// A core-side job passes no slug and must stay ungated
func TestUnattributedJobStaysUngated(t *testing.T) {
	disableAll(t)
	ran := false
	if err := gatedJobHandler("", func(context.Context) error { ran = true; return nil })(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Error("a job with no plugin slug was gated")
	}
}

// A plugin an operator turned off must not be able to 503 /readyz and pull the pod from rotation
func TestDisabledPluginProbeIsNotRun(t *testing.T) {
	healthRegistry.mu.Lock()
	healthRegistry.health = nil
	healthRegistry.mu.Unlock()
	t.Cleanup(func() {
		healthRegistry.mu.Lock()
		healthRegistry.health = nil
		healthRegistry.mu.Unlock()
	})

	h := &Host{}
	failing := func(context.Context) error { return context.DeadlineExceeded }
	_ = h.RegisterHealthCheck("off-plugin", "api", sdk.HealthCheck(failing))
	disableAll(t)

	rec := httptest.NewRecorder()
	HealthHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))

	if rec.Code != 200 {
		t.Errorf("a disabled plugin's failing probe took /healthz down: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "off-plugin") {
		t.Errorf("disabled plugin still appears in the health report: %s", rec.Body.String())
	}
}
