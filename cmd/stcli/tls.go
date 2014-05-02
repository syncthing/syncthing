package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base32"
	"path/filepath"
	"strings"
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
