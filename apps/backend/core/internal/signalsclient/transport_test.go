package signalsclient

import (
	"context"
	"crypto/tls"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testtls"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func TestTransportFromEnv(t *testing.T) {
	ca := filepath.Join(t.TempDir(), "ca.pem")
	for name, c := range map[string]struct {
		env  map[string]string
		want Transport
		err  string
	}{
		"unset":          {nil, Transport{}, ""},
		"on":             {map[string]string{EnvTLS: "true"}, Transport{TLS: true}, ""},
		"off":            {map[string]string{EnvTLS: "false"}, Transport{}, ""},
		"a private CA":   {map[string]string{EnvTLS: "1", EnvTLSCA: " " + ca}, Transport{TLS: true, CAFile: ca}, ""},
		"not a bool":     {map[string]string{EnvTLS: "yes"}, Transport{}, "takes true or false"},
		"CA without TLS": {map[string]string{EnvTLSCA: ca}, Transport{}, "isn't true"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := TransportFromEnv(func(k string) string { return c.env[k] })
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("err = %v, want %q", err, c.err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %+v, %v - want %+v", got, err, c.want)
			}
		})
	}
}

type ingestOK struct {
	signalsv1.UnimplementedIngestServiceServer
}

func (ingestOK) RecordActivity(context.Context, *signalsv1.RecordActivityRequest) (*signalsv1.RecordActivityResponse, error) {
	return &signalsv1.RecordActivityResponse{}, nil
}

func TestTheContractRunsOverTLS(t *testing.T) {
	files := testtls.Write(t, t.TempDir(), "localhost", "127.0.0.1")
	pair, err := tls.LoadX509KeyPair(files.Cert, files.Key)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})))
	signalsv1.RegisterIngestServiceServer(srv, ingestOK{})
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	addr := ln.Addr().String() // the address it listens on - "localhost" can resolve to ::1 first and be refused, not verified
	call := func(tr Transport) error {
		c, err := New(addr, tr, auth.MustServiceKeys("test-secret"), time.Minute, nil)
		if err != nil {
			return err
		}
		defer c.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, err = c.Ingest.RecordActivity(ctx, &signalsv1.RecordActivityRequest{})
		return err
	}
	if err := call(Transport{TLS: true, CAFile: files.CA}); err != nil {
		t.Fatalf("over TLS: %v", err)
	}
	if err := call(Transport{}); status.Code(err) != codes.Unavailable {
		t.Errorf("plaintext against a TLS suite: %v", err)
	}
	if err := call(Transport{TLS: true}); status.Code(err) != codes.Unavailable || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("trusting only the system roots: %v", err)
	}
	stranger := testtls.Write(t, t.TempDir(), "localhost")
	if err := call(Transport{TLS: true, CAFile: stranger.CA}); status.Code(err) != codes.Unavailable {
		t.Errorf("another CA's certificate: %v", err)
	}
	if err := call(Transport{TLS: true, CAFile: files.Key}); err == nil || !strings.Contains(err.Error(), "no PEM certificate") {
		t.Errorf("a CA file holding a key: %v", err)
	}
}
