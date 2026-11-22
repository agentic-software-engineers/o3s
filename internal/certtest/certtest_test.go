package certtest_test

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	"github.com/hansehart/o3s/internal/certtest"
)

// The CA is a root its pool trusts, made to sign server certificates.
func TestCAIsARootItsPoolTrusts(t *testing.T) {
	ca, roots := certtest.CA(t)

	_, err := ca.Leaf.Verify(x509.VerifyOptions{Roots: roots})

	// Require a root that verifies against its own pool and signs certificates
	if err != nil || !ca.Leaf.IsCA || ca.Leaf.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Errorf("CA verifies = %v, is a CA %v, signs %v, want a signing root", err, ca.Leaf.IsCA, ca.Leaf.KeyUsage&x509.KeyUsageCertSign != 0)
	}
}

// The CA's PEM holds its key and its certificate in the one file o3s keeps for a home's proxy.
func TestPEMLoadsBackAsTheSameCA(t *testing.T) {
	ca, _ := certtest.CA(t)
	pem := certtest.PEM(t, ca)

	got, err := tls.X509KeyPair(pem, pem)

	// Require the same certificate back with its key
	if err != nil || len(got.Certificate) != 1 || string(got.Certificate[0]) != string(ca.Certificate[0]) {
		t.Errorf("X509KeyPair(PEM) = %v, want the CA's own certificate and key", err)
	}
}
