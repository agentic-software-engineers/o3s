// Package certtest hands tests a CA shaped like the one o3s keeps for a home's proxy, and the PEM file o3s keeps it
// in.
package certtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

// CA makes a root that signs server certificates, and a pool that trusts it.
func CA(tb testing.TB) (tls.Certificate, *x509.CertPool) {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	// Stop when the key fails to generate
	if err != nil {
		tb.Fatalf("Setup: GenerateKey = %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"o3s"}, CommonName: "Egress Proxy CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	// Stop when the CA fails to sign
	if err != nil {
		tb.Fatalf("Setup: CreateCertificate = %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	// Stop when the CA fails to parse back
	if err != nil {
		tb.Fatalf("Setup: ParseCertificate = %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, roots
}

// PEM writes the CA's key and then its certificate, the way o3s keeps them in one file.
func PEM(tb testing.TB, ca tls.Certificate) []byte {
	tb.Helper()
	key, err := x509.MarshalPKCS8PrivateKey(ca.PrivateKey)
	// Stop when the key fails to encode
	if err != nil {
		tb.Fatalf("Setup: MarshalPKCS8PrivateKey = %v", err)
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})
	return append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Certificate[0]})...)
}
