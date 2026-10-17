package signalsclient

import (
	"strings"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// dns, not passthrough - it re-resolves on reconnect and balances across every replica the name answers with
func TestTheClientBalancesAcrossReplicas(t *testing.T) {
	c, err := New("signals:8092", auth.MustServiceKeys("test-secret"), time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if !strings.HasPrefix(c.conn.Target(), "dns:///") {
		t.Fatalf("target %q, want the dns resolver", c.conn.Target())
	}
}
