package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base32"
	"encoding/binary"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	tlsRSABits = 3072
	tlsName    = "syncthing"
)

func loadCert(dir string) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"))
}

func certID(bs []byte) string {
	hf := sha256.New()
	hf.Write(bs)
	id := hf.Sum(nil)
	return strings.Trim(base32.StdEncoding.EncodeToString(id), "=")
}

func certSeed(bs []byte) int64 {
	hf := sha256.New()
	hf.Write(bs)
	id := hf.Sum(nil)
	return int64(binary.BigEndian.Uint64(id))
}

func newCertificate(dir string) {
	infoln("Generating RSA certificate and key...")

	priv, err := rsa.GenerateKey(rand.Reader, tlsRSABits)
	fatalErr(err)

	notBefore := time.Now()
	notAfter := time.Date(2049, 12, 31, 23, 59, 59, 0, time.UTC)

	template := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
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
	fatalErr(err)

	certOut, err := os.Create(filepath.Join(dir, "cert.pem"))
	fatalErr(err)
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()
	okln("Created RSA certificate file")

	keyOut, err := os.OpenFile(filepath.Join(dir, "key.pem"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	fatalErr(err)
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()
	okln("Created RSA key file")
}
