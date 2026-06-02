package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

type linkerdCerts struct {
	trustAnchorPEM string
	issuerCertPEM  string
	issuerKeyPEM   string
}

// generateLinkerdCerts mirrors the terraform tls_* resources: a self-signed
// 10-year root trust anchor and a 1-year issuer intermediate signed by it.
func generateLinkerdCerts() (linkerdCerts, error) {
	anchorKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return linkerdCerts{}, fmt.Errorf("anchor key: %w", err)
	}

	anchorTmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "root.linkerd.cluster.local"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(87600 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	anchorDER, err := x509.CreateCertificate(rand.Reader, anchorTmpl, anchorTmpl, &anchorKey.PublicKey, anchorKey)
	if err != nil {
		return linkerdCerts{}, fmt.Errorf("anchor cert: %w", err)
	}
	anchorCert, err := x509.ParseCertificate(anchorDER)
	if err != nil {
		return linkerdCerts{}, fmt.Errorf("parse anchor: %w", err)
	}

	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return linkerdCerts{}, fmt.Errorf("issuer key: %w", err)
	}

	issuerTmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "identity.linkerd.cluster.local"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(8760 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	issuerDER, err := x509.CreateCertificate(rand.Reader, issuerTmpl, anchorCert, &issuerKey.PublicKey, anchorKey)
	if err != nil {
		return linkerdCerts{}, fmt.Errorf("issuer cert: %w", err)
	}

	issuerKeyDER, err := x509.MarshalECPrivateKey(issuerKey)
	if err != nil {
		return linkerdCerts{}, fmt.Errorf("marshal issuer key: %w", err)
	}

	return linkerdCerts{
		trustAnchorPEM: pemEncode("CERTIFICATE", anchorDER),
		issuerCertPEM:  pemEncode("CERTIFICATE", issuerDER),
		issuerKeyPEM:   pemEncode("EC PRIVATE KEY", issuerKeyDER),
	}, nil
}

func pemEncode(typ string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}))
}

func serial() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	n, _ := rand.Int(rand.Reader, limit)
	return n
}
