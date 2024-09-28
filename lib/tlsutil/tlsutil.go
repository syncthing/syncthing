// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/syncthing/syncthing/lib/rand"
)

var (
	ErrIdentificationFailed = errors.New("failed to identify socket type")

	// The list of cipher suites we will use / suggest for TLS 1.2 connections.
	cipherSuites = []uint16{
		// Suites that are good and fast on hardware *without* AES-NI.
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,

		// Suites that are good and fast on hardware with AES-NI. These are
		// reordered from the Go default to put the 256 bit ciphers above the
		// 128 bit ones - because that looks cooler, even though there is
		// probably no relevant difference in strength yet.
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,

		// The rest of the suites, minus DES stuff.
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
		tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	}
)

// SecureDefault returns a tls.Config with reasonable, secure defaults set.
// This variant allows only TLS 1.3.
func SecureDefaultTLS13() *tls.Config {
	return &tls.Config{
		// TLS 1.3 is the minimum we accept
		MinVersion:         tls.VersionTLS13,
		ClientSessionCache: tls.NewLRUClientSessionCache(0),
	}
}

// SecureDefaultWithTLS12 returns a tls.Config with reasonable, secure
// defaults set. This variant allows TLS 1.2.
func SecureDefaultWithTLS12() *tls.Config {
	// paranoia
	cs := make([]uint16, len(cipherSuites))
	copy(cs, cipherSuites)

	return &tls.Config{
		// TLS 1.2 is the minimum we accept
		MinVersion: tls.VersionTLS12,
		// The cipher suite lists built above. These are ignored in TLS 1.3.
		CipherSuites: cs,
		// We've put some thought into this choice and would like it to
		// matter.
		PreferServerCipherSuites: true,

		ClientSessionCache: tls.NewLRUClientSessionCache(0),
	}
}

// generateCertificate generates a PEM formatted key pair and self-signed certificate in memory.
func generateCertificate(commonName string, lifetimeDays int) (*pem.Block, *pem.Block, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	notBefore := time.Now().Truncate(24 * time.Hour)
	notAfter := notBefore.Add(time.Duration(lifetimeDays*24) * time.Hour)

	// NOTE: update lib/api.shouldRegenerateCertificate() appropriately if
	// you add or change attributes in here, especially DNSNames or
	// IPAddresses.
	template := x509.Certificate{
		SerialNumber: new(big.Int).SetUint64(rand.Uint64()),
		Subject: pkix.Name{
			CommonName:         commonName,
			Organization:       []string{"Syncthing"},
			OrganizationalUnit: []string{"Automatically Generated"},
		},
		DNSNames:              []string{commonName},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create cert: %w", err)
	}

	certBlock := &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}
	keyBlock, err := pemBlockForKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("save key: %w", err)
	}

	return certBlock, keyBlock, nil
}

// NewCertificate generates and returns a new TLS certificate, saved to the given PEM files.
func NewCertificate(certFile, keyFile string, commonName string, lifetimeDays int) (tls.Certificate, error) {
	certBlock, keyBlock, err := generateCertificate(commonName, lifetimeDays)
	if err != nil {
		return tls.Certificate{}, err
	}

	certOut, err := os.Create(certFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("save cert: %w", err)
	}
	if err = pem.Encode(certOut, certBlock); err != nil {
		return tls.Certificate{}, fmt.Errorf("save cert: %w", err)
	}
	if err = certOut.Close(); err != nil {
		return tls.Certificate{}, fmt.Errorf("save cert: %w", err)
	}

	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("save key: %w", err)
	}
	if err = pem.Encode(keyOut, keyBlock); err != nil {
		return tls.Certificate{}, fmt.Errorf("save key: %w", err)
	}
	if err = keyOut.Close(); err != nil {
		return tls.Certificate{}, fmt.Errorf("save key: %w", err)
	}

	return tls.X509KeyPair(pem.EncodeToMemory(certBlock), pem.EncodeToMemory(keyBlock))
}

// NewCertificateInMemory generates and returns a new TLS certificate, kept only in memory.
func NewCertificateInMemory(commonName string, lifetimeDays int) (tls.Certificate, error) {
	certBlock, keyBlock, err := generateCertificate(commonName, lifetimeDays)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(pem.EncodeToMemory(certBlock), pem.EncodeToMemory(keyBlock))
}

type DowngradingListener struct {
	net.Listener
	TLSConfig *tls.Config
}

func (l *DowngradingListener) Accept() (net.Conn, error) {
	conn, isTLS, err := l.AcceptNoWrapTLS()

	// We failed to identify the socket type, pretend that everything is fine,
	// and pass it to the underlying handler, and let them deal with it.
	if err == ErrIdentificationFailed {
		return conn, nil
	}

	if err != nil {
		return conn, err
	}

	if isTLS {
		return tls.Server(conn, l.TLSConfig), nil
	}
	return conn, nil
}

func (l *DowngradingListener) AcceptNoWrapTLS() (net.Conn, bool, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, false, err
	}

	union := &UnionedConnection{Conn: conn}

	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, err := conn.Read(union.first[:])
	conn.SetReadDeadline(time.Time{})
	if err != nil || n == 0 {
		// We hit a read error here, but the Accept() call succeeded so we must not return an error.
		// We return the connection as is with a special error which handles this
		// special case in Accept().
		return conn, false, ErrIdentificationFailed
	}

	return union, union.first[0] == 0x16, nil
}

type UnionedConnection struct {
	first     [1]byte
	firstDone bool
	net.Conn
}

func (c *UnionedConnection) Read(b []byte) (n int, err error) {
	if !c.firstDone {
		if len(b) == 0 {
			// this probably doesn't happen, but handle it anyway
			return 0, nil
		}
		b[0] = c.first[0]
		c.firstDone = true
		return 1, nil
	}
	return c.Conn.Read(b)
}

func pemBlockForKey(priv interface{}) (*pem.Block, error) {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}, nil
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}, nil
	default:
		return nil, errors.New("unknown key type")
	}
}
