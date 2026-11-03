package signalsgrpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/testtls"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestTLSFromEnv(t *testing.T) {
	f := testtls.Write(t, t.TempDir(), "signals")
	if creds, err := TLSFromEnv(envOf(nil)); creds != nil || err != nil {
		t.Errorf("neither file: %v, %v - want plaintext", creds, err)
	}
	if _, err := TLSFromEnv(envOf(map[string]string{EnvTLSCert: f.Cert})); err == nil || !strings.Contains(err.Error(), "go together") {
		t.Errorf("a certificate without its key: %v", err)
	}
	if _, err := TLSFromEnv(envOf(map[string]string{EnvTLSCert: f.Cert, EnvTLSKey: f.Cert})); err == nil {
		t.Error("a pair that doesn't load was accepted at boot")
	}
	if creds, err := TLSFromEnv(envOf(map[string]string{EnvTLSCert: f.Cert, EnvTLSKey: f.Key})); creds == nil || err != nil {
		t.Errorf("a good pair: %v, %v", creds, err)
	}
}

func TestARenewedCertificateIsServedWithoutARestart(t *testing.T) {
	dir := t.TempDir()
	first := testtls.Write(t, dir, "signals")
	r := &certReloader{certFile: first.Cert, keyFile: first.Key}
	served := func() []byte {
		t.Helper()
		c, err := r.get(nil)
		if err != nil {
			t.Fatal(err)
		}
		return c.Certificate[0]
	}
	before := served()
	bump := func(at time.Time) {
		for _, p := range []string{first.Cert, first.Key} {
			if err := os.Chtimes(p, at, at); err != nil {
				t.Fatal(err)
			}
		}
	}
	testtls.Write(t, dir, "signals")
	bump(time.Now().Add(time.Minute))
	renewed := served()
	if bytes.Equal(before, renewed) {
		t.Fatal("still serving the certificate from before the renewal")
	}
	if err := os.WriteFile(first.Key, []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}
	bump(time.Now().Add(2 * time.Minute))
	if !bytes.Equal(served(), renewed) {
		t.Error("a renewal that doesn't load replaced the last good pair")
	}
	if err := os.Remove(first.Cert); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(served(), renewed) {
		t.Error("a file missing mid-swap dropped the last good pair")
	}
}

func TestTheContractServesOnlyTLSWhenGivenAPair(t *testing.T) {
	f := testtls.Write(t, t.TempDir(), "localhost", "127.0.0.1")
	creds, err := TLSFromEnv(envOf(map[string]string{EnvTLSCert: f.Cert, EnvTLSKey: f.Key}))
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(Deps{ServiceKeys: testKeys, Creds: creds})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	ca, err := os.ReadFile(f.CA)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	check := func(tc credentials.TransportCredentials) (*healthpb.HealthCheckResponse, error) {
		conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(tc))
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	}
	if res, err := check(credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12})); err != nil || res.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("over TLS: %v, %v", res, err)
	}
	if _, err := check(insecure.NewCredentials()); status.Code(err) != codes.Unavailable {
		t.Errorf("a plaintext caller got through: %v", err)
	}
}
