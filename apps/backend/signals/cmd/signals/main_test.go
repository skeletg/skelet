package main

import (
	"context"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestTrustedProxiesLetTheShoppersIPThroughCoresProxy(t *testing.T) {
	t.Cleanup(func() { _ = auth.SetTrustedProxies(nil) })
	env := map[string]string{"TRUSTED_PROXIES": "10.0.0.0/8, 192.168.1.5"}
	if err := trustProxies(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("trustProxies: %v", err)
	}
	r := httptest.NewRequest("POST", "/t", nil)
	r.RemoteAddr = "10.1.2.3:41000"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := auth.ClientIP(r); got != "203.0.113.9" {
		t.Errorf("ClientIP behind a trusted proxy = %q, want the forwarded client", got)
	}
	r.RemoteAddr = "198.51.100.7:41000"
	if got := auth.ClientIP(r); got != "198.51.100.7" {
		t.Errorf("ClientIP from an untrusted peer = %q, want the peer itself", got)
	}
	env["TRUSTED_PROXIES"] = "not-a-cidr"
	if err := trustProxies(func(k string) string { return env[k] }); err == nil {
		t.Error("a malformed TRUSTED_PROXIES was accepted")
	}
}

func TestTheGRPCDrainIsBoundedByAStuckCall(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })
	watch, err := healthpb.NewHealthClient(cc).Watch(context.Background(), &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	if _, err := watch.Recv(); err != nil {
		t.Fatalf("first health update: %v", err)
	}

	done := make(chan struct{})
	go func() {
		stopGRPC(srv, 50*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stopGRPC waited on an in-flight stream past its bound")
	}
}
