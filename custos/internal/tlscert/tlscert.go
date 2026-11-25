// Package tlscert creates custosd's self-signed certificate on first start
// (on /data, never in the image: every device has its own key).
package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Ensure loads dir/cert.pem + dir/key.pem, creating them if missing.
func Ensure(dir string, names []string, ips []net.IP) (tls.Certificate, error) {
	cp, kp := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if c, err := tls.LoadX509KeyPair(cp, kp); err == nil {
		return c, nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return tls.Certificate{}, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "custos.local", Organization: []string{"Custos Marginis"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              names,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	if err := os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(cp, kp)
}
