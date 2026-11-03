// Package testtls writes a throwaway CA and a server certificate it signed into a dir - writing again is a renewal
package testtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type Files struct{ CA, Cert, Key string }

func Write(t testing.TB, dir string, hosts ...string) Files {
	t.Helper()
	now := time.Now()
	caKey := key(t)
	ca := &x509.Certificate{
		SerialNumber: serial(t), Subject: pkix.Name{CommonName: "skelet test CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	if ca, err = x509.ParseCertificate(caDER); err != nil {
		t.Fatal(err)
	}
	leafKey := key(t)
	leaf := &x509.Certificate{
		SerialNumber: serial(t), Subject: pkix.Name{CommonName: hosts[0]},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			leaf.IPAddresses = append(leaf.IPAddresses, ip)
		} else {
			leaf.DNSNames = append(leaf.DNSNames, h)
		}
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	f := Files{CA: filepath.Join(dir, "ca.pem"), Cert: filepath.Join(dir, "cert.pem"), Key: filepath.Join(dir, "key.pem")}
	writePEM(t, f.CA, "CERTIFICATE", caDER)
	writePEM(t, f.Cert, "CERTIFICATE", leafDER)
	writePEM(t, f.Key, "PRIVATE KEY", keyDER)
	return f
}

func key(t testing.TB) *ecdsa.PrivateKey {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func serial(t testing.TB) *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func writePEM(t testing.TB, path, kind string, der []byte) {
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}
