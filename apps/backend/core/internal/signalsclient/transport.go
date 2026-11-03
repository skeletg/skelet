package signalsclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	EnvTLS   = "SIGNALS_GRPC_TLS"
	EnvTLSCA = "SIGNALS_GRPC_TLS_CA"
)

// Transport is how core reaches the suite - the zero value is plaintext
type Transport struct {
	TLS    bool
	CAFile string
}

func TransportFromEnv(getenv func(string) string) (Transport, error) {
	t := Transport{CAFile: strings.TrimSpace(getenv(EnvTLSCA))}
	if v := strings.TrimSpace(getenv(EnvTLS)); v != "" {
		on, err := strconv.ParseBool(v)
		if err != nil {
			return Transport{}, fmt.Errorf("%s=%s - it takes true or false", EnvTLS, v)
		}
		t.TLS = on
	}
	if t.CAFile != "" && !t.TLS {
		return Transport{}, fmt.Errorf("%s is set but %s isn't true - turn TLS on, or drop the CA", EnvTLSCA, EnvTLS)
	}
	return t, nil
}

func (t Transport) credentials() (credentials.TransportCredentials, error) {
	if !t.TLS {
		return insecure.NewCredentials(), nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if t.CAFile != "" {
		pemBytes, err := os.ReadFile(t.CAFile)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", EnvTLSCA, err)
		}
		cfg.RootCAs = x509.NewCertPool()
		if !cfg.RootCAs.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("%s: %s holds no PEM certificate", EnvTLSCA, t.CAFile)
		}
	}
	return credentials.NewTLS(cfg), nil
}
