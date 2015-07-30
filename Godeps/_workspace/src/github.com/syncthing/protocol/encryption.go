// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package protocol

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/rand"
	"crypto/sha256"
)

func Encrypt(buf []byte, label []byte, cert tls.Certificate) (out []byte, err error) {
	var ret []byte

	l.Debugf("Trying to encrypt", len(buf), "bytes of data")

	// Certificate stuff
	pub, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		l.Debugln("error:", err)
		return nil, err
	}

	pubkey := pub.PublicKey.(*rsa.PublicKey)

	// now to encrypting
	// each encrypted chunk may only be ((pubkey.N.BitLen() + 7) / 8) - 11 byte big, so we may have to cut here

	sha := sha256.New()

	k := ((pubkey.N.BitLen() + 7) / 8) -2*sha.Size()-2

	var offset int
	
	for i := 0; i < len(buf); i += k {
		l.Debugf("Encrypt cicle i", i, "k", k, "i+k", i+k, "len", len(buf))
		if i + k > len(buf) {
			k = len(buf) - i
		}
		out, err := rsa.EncryptOAEP(sha, rand.Reader, pubkey, buf[i:i+k], label) // Outputs 384 Bytes
		if err != nil {
			l.Debugln("error:", err)
			return nil, err
		}

		ret = append(ret, out...)

		offset += len(out)
	}

	return ret, nil
}

func Decrypt(buf []byte, label []byte, key *rsa.PrivateKey) (out []byte, err error) {
	var ret []byte

	l.Debugf("Trying to decrypt", len(buf), "bytes of data")

	// now to encrypting
	// each encrypted chunk may only be ((pubkey.N.BitLen() + 7) / 8) - 11 byte big, so we may have to cut her

	k := 384

	var offset int
	
	for i := 0; i < len(buf); i += k {
		l.Debugf("Decrypt cicle i", i, "k", k, "i+k", i+k, "len", len(buf))
		if i + k > len(buf) {
			k = len(buf) - i
		}
		out, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, buf[i:i+k], label) // Outputs 384 Bytes
		if err != nil {
			l.Debugln("error:", err)
			return nil, err
		}

		ret = append(ret, out...)

		offset += len(out)
	}

	return ret, nil
}