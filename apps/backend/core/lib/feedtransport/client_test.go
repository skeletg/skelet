package feedtransport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"github.com/skeletg/skelet/apps/backend/sdk"
	"golang.org/x/crypto/ssh"
)

func testSFTP(t *testing.T) (string, string, *atomic.Int64) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	auths := &atomic.Int64{}
	cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		auths.Add(1)
		if string(password) != "test-password" {
			return nil, errors.New("invalid password")
		}
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				defer conn.Close()
				sc, channels, requests, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				defer sc.Close()
				go ssh.DiscardRequests(requests)
				for newch := range channels {
					if newch.ChannelType() != "session" {
						_ = newch.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					ch, reqs, err := newch.Accept()
					if err != nil {
						return
					}
					go func() {
						for req := range reqs {
							_ = req.Reply(req.Type == "subsystem", nil)
						}
					}()
					server, err := sftp.NewServer(ch)
					if err != nil {
						return
					}
					_ = server.Serve()
					_ = server.Close()
				}
			})
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done; wg.Wait() })
	return listener.Addr().String(), ssh.FingerprintSHA256(signer.PublicKey()), auths
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestSFTPUploadPinsTheServerAndReplacesOnlyCompleteFiles(t *testing.T) {
	address, fingerprint, auths := testSFTP(t)
	target := sdk.SFTPTarget{Address: address, Username: "merchant", Secret: "test-password", HostKeySHA256: fingerprint, Path: filepath.Join(t.TempDir(), "catalog.jsonl.gz")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := New(true)
	for _, body := range []string{"first snapshot", "replacement snapshot"} {
		if err := client.UploadSFTP(ctx, target, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(target.Path); err != nil || string(b) != body {
			t.Fatalf("uploaded %q: %v", b, err)
		}
	}
	if err := client.UploadSFTP(ctx, target, io.MultiReader(strings.NewReader("partial"), failingReader{})); err == nil {
		t.Fatal("failed body reported success")
	}
	if b, _ := os.ReadFile(target.Path); string(b) != "replacement snapshot" {
		t.Fatalf("failed upload replaced good data: %q", b)
	}
	if files, _ := filepath.Glob(filepath.Join(filepath.Dir(target.Path), ".*.tmp")); len(files) != 0 {
		t.Fatalf("temporary uploads left behind: %v", files)
	}
	before := auths.Load()
	target.HostKeySHA256 = "SHA256:untrusted"
	if err := client.UploadSFTP(ctx, target, strings.NewReader("bad")); !sdk.IsPermanent(err) {
		t.Fatalf("host mismatch was not permanent: %v", err)
	}
	if auths.Load() != before {
		t.Fatal("credentials sent before verifying the server")
	}
	target.HostKeySHA256 = fingerprint
	if err := New(false).UploadSFTP(ctx, target, strings.NewReader("bad")); err == nil {
		t.Fatal("production transport allowed a private target")
	}
}

func TestHTTPRejectsCredentialURLsAndDoesNotFollowUploadRedirects(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer server.Close()
	client := New(true)
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("snapshot"))
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if calls != 1 || res.StatusCode != http.StatusFound {
		t.Fatal("upload redirected instead of reporting failure")
	}
	req.URL.User = url.UserPassword("merchant", "secret")
	res, err = client.Do(req)
	if res != nil {
		_ = res.Body.Close()
	}
	if !sdk.IsPermanent(err) || calls != 1 {
		t.Fatalf("credential URL reached the server: %v", err)
	}
}
