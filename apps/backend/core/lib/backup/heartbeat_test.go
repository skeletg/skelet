package backup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHeartbeatPingsConfiguredURL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
	}))
	defer srv.Close()

	s := &Service{cfg: envConfig{HeartbeatURL: srv.URL}}
	s.Heartbeat(context.Background())
	if got := hits.Load(); got != 1 {
		t.Fatalf("pings = %d, want 1", got)
	}
}

// Unset is the default, and pinging nothing must stay silent rather than erroring
func TestHeartbeatUnsetIsNoOp(t *testing.T) {
	s := &Service{cfg: envConfig{}}
	s.Heartbeat(context.Background()) // would panic or block if it tried to send
}

// A monitoring outage must not look like a backup failure - the dump is already uploaded
// by the time this runs, so every unhappy path here is a warning and nothing more
func TestHeartbeatFailuresAreNotFatal(t *testing.T) {
	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer rejecting.Close()

	for name, url := range map[string]string{
		"rejected":    rejecting.URL,
		"unreachable": "http://127.0.0.1:1/nope",
		"unparseable": "http://[::1]:namedport/x",
	} {
		t.Run(name, func(t *testing.T) {
			s := &Service{cfg: envConfig{HeartbeatURL: url}}
			s.Heartbeat(context.Background())
		})
	}
}
