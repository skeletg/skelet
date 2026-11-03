package signalsgrpc

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/credentials"
)

const (
	EnvTLSCert = "SIGNALS_GRPC_TLS_CERT"
	EnvTLSKey  = "SIGNALS_GRPC_TLS_KEY"
)

func TLSFromEnv(getenv func(string) string) (credentials.TransportCredentials, error) {
	cert, key := strings.TrimSpace(getenv(EnvTLSCert)), strings.TrimSpace(getenv(EnvTLSKey))
	switch {
	case cert == "" && key == "":
		return nil, nil
	case cert == "" || key == "":
		return nil, fmt.Errorf("%s and %s go together - set both to serve TLS, or neither for plaintext", EnvTLSCert, EnvTLSKey)
	}
	r := &certReloader{certFile: cert, keyFile: key}
	if _, err := r.get(nil); err != nil {
		return nil, fmt.Errorf("%s, %s: %w", EnvTLSCert, EnvTLSKey, err)
	}
	return credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: r.get}), nil
}

// certReloader re-reads the pair whenever either file changes, so a renewed certificate is served without a restart
type certReloader struct {
	certFile, keyFile string

	mu         sync.Mutex
	stamp, bad [2]time.Time
	cert       *tls.Certificate
}

func (r *certReloader) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ci, cerr := os.Stat(r.certFile)
	ki, kerr := os.Stat(r.keyFile)
	err := errors.Join(cerr, kerr)
	if err == nil {
		if stamp := [2]time.Time{ci.ModTime(), ki.ModTime()}; r.cert == nil || stamp != r.stamp {
			var c tls.Certificate
			if c, err = tls.LoadX509KeyPair(r.certFile, r.keyFile); err == nil {
				r.cert, r.stamp = &c, stamp
			} else if r.cert != nil && stamp != r.bad {
				r.bad = stamp
				slog.Warn("signals contract: still serving the previous TLS certificate - the new pair doesn't load", "err", err)
			}
		}
	}
	if r.cert == nil {
		return nil, err
	}
	return r.cert, nil
}
