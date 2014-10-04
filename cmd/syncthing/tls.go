// Copyright (C) 2014 Jakob Borg and Contributors (see the CONTRIBUTORS file).
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU General Public License as published by the Free
// Software Foundation, either version 3 of the License, or (at your option)
// any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
// more details.
//
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"io"
	"math/big"
	mr "math/rand"
	"net"
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
	if err != nil {
		l.Fatalln("generate key:", err)
	}

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
	if err != nil {
		l.Fatalln("create cert:", err)
	}

	certOut, err := os.Create(filepath.Join(dir, prefix+"cert.pem"))
	if err != nil {
		l.Fatalln("save cert:", err)
	}
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		l.Fatalln("save cert:", err)
	}
	err = certOut.Close()
	if err != nil {
		l.Fatalln("save cert:", err)
	}

	keyOut, err := os.OpenFile(filepath.Join(dir, prefix+"key.pem"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		l.Fatalln("save key:", err)
	}
	err = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	if err != nil {
		l.Fatalln("save key:", err)
	}
	err = keyOut.Close()
	if err != nil {
		l.Fatalln("save key:", err)
	}
}

type DowngradingListener struct {
	net.Listener
	TLSConfig *tls.Config
}

type WrappedConnection struct {
	io.Reader
	net.Conn
}

func (l *DowngradingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	br := bufio.NewReader(conn)
	bs, err := br.Peek(1)
	if err != nil {
		// We hit a read error here, but the Accept() call succeeded so we must not return an error.
		// We return the connection as is and let whoever tries to use it deal with the error.
		return conn, nil
	}

	wrapper := &WrappedConnection{br, conn}

	// 0x16 is the first byte of a TLS handshake
	if bs[0] == 0x16 {
		return tls.Server(wrapper, l.TLSConfig), nil
	}

	return wrapper, nil
}

func (c *WrappedConnection) Read(b []byte) (n int, err error) {
	return c.Reader.Read(b)
}
