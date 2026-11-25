// Package wgkeys creates WireGuard key pairs with the Go standard library
// (X25519), so the device needs neither wg genkey nor Python.
package wgkeys

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// Generate returns a new (private, public) key pair, base64 like wg(8).
func Generate() (priv, pub string, err error) {
	var k [32]byte
	if _, err = rand.Read(k[:]); err != nil {
		return "", "", err
	}
	// Clamp like wg genkey (the curve does it anyway; this keeps the
	// printed key identical to what WireGuard uses).
	k[0] &= 248
	k[31] = (k[31] & 127) | 64
	return fromBytes(k[:])
}

// Public derives the public key from a base64 private key.
func Public(priv string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(priv)
	if err != nil || len(b) != 32 {
		return "", fmt.Errorf("invalid WireGuard private key")
	}
	_, pub, err := fromBytes(b)
	return pub, err
}

func fromBytes(k []byte) (string, string, error) {
	p, err := ecdh.X25519().NewPrivateKey(k)
	if err != nil {
		return "", "", err
	}
	enc := base64.StdEncoding.EncodeToString
	return enc(k), enc(p.PublicKey().Bytes()), nil
}
