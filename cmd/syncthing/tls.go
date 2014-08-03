// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
// All rights reserved. Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	mr "math/rand"
	"os"
	"path/filepath"
	"time"
)

const (
	tlsRSABits = 3072
	tlsName    = "syncthing"
)

func loadCert(dir string, prefix string) (tls.Certificate, error) {
	cf := filepath.Join(dir, prefix+"cert.pem")
	kf := filepath.Join(dir, prefix+"key.pem")
	return tls.LoadX509KeyPair(cf, kf)
}

func certSeed(bs []byte) int64 {
	hf := sha256.New()
	hf.Write(bs)
	id := hf.Sum(nil)
	return int64(binary.BigEndian.Uint64(id))
}

func newCertificate(dir string, prefix string) {
	l.Infoln("Generating RSA key and certificate...")

	priv, err := rsa.GenerateKey(rand.Reader, tlsRSABits)
	l.FatalErr(err)

	notBefore := time.Now()
	notAfter := time.Date(2049, 12, 31, 23, 59, 59, 0, time.UTC)

	template := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(mr.Int63()),
		Subject: pkix.Name{
			CommonName: tlsName,
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	l.FatalErr(err)

	certOut, err := os.Create(filepath.Join(dir, prefix+"cert.pem"))
	l.FatalErr(err)
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, err := os.OpenFile(filepath.Join(dir, prefix+"key.pem"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	l.FatalErr(err)
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
}
