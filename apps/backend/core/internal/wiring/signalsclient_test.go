package wiring

import (
	"strings"
	"testing"

	"go.uber.org/fx/fxtest"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestTheContractsTLSSettingCountsOnlyWithASuiteToDial(t *testing.T) {
	cfg := Config{SignalsServiceKeys: auth.MustServiceKeys("test-secret")}
	t.Setenv("SIGNALS_GRPC_ADDR", "signals:8092")
	t.Setenv("SIGNALS_GRPC_TLS", "yes")
	if _, err := newSignalsClient(fxtest.NewLifecycle(t), cfg, true, nil); err == nil || !strings.Contains(err.Error(), "SIGNALS_GRPC_TLS") {
		t.Errorf("a bad setting with a suite to dial booted: %v", err)
	}
	if c, err := newSignalsClient(fxtest.NewLifecycle(t), cfg, false, nil); c != nil || err != nil {
		t.Errorf("switched off: %v, %v", c, err)
	}
	t.Setenv("SIGNALS_GRPC_TLS", "true")
	c, err := newSignalsClient(fxtest.NewLifecycle(t), cfg, true, nil)
	if c == nil || err != nil {
		t.Fatalf("TLS on: %v, %v", c, err)
	}
	_ = c.Close()
}
