// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at http://mozilla.org/MPL/2.0/.

package protocol

import (
	"crypto/cipher"
	"crypto/aes"
	"crypto/sha1"
	"golang.org/x/crypto/pbkdf2"
)

func Encrypt(buf []byte, passphrase string, salt string) (out []byte, err error) {
	key := pbkdf2.Key([]byte(passphrase), []byte(salt), 4096, 32, sha1.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// If the key is unique for each ciphertext, then it's ok to use a zero IV
	var iv [aes.BlockSize]byte
	stream := cipher.NewCFBEncrypter(block, iv[:])
	stream.XORKeyStream(buf, buf)

	return buf, nil
}

func Decrypt(buf []byte, passphrase string, salt string) (out []byte, err error) {
	key := pbkdf2.Key([]byte(passphrase), []byte(salt), 4096, 32, sha1.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// If the key is unique for each ciphertext, then it's ok to use a zero IV
	var iv [aes.BlockSize]byte
	stream := cipher.NewCFBDecrypter(block, iv[:])
	stream.XORKeyStream(buf, buf)

	return buf, nil
}

// func Encrypt(buf []byte, label []byte, cert tls.Certificate) (out []byte, err error) {
// 	var ret []byte

// 	// Certificate stuff
// 	pub, err := x509.ParseCertificate(cert.Certificate[0])
// 	if err != nil {
// 		l.Debugln("error:", err)
// 		return nil, err
// 	}

// 	pubkey := pub.PublicKey.(*rsa.PublicKey)

// 	// now to encrypting
// 	// each encrypted chunk may only be ((pubkey.N.BitLen() + 7) / 8) - 11 byte big, so we may have to cut here

// 	sha := sha256.New()

// 	k := ((pubkey.N.BitLen() + 7) / 8) -2*sha.Size()-2

// 	var offset int
	
// 	for i := 0; i < len(buf); i += k {
// 		if i + k > len(buf) {
// 			k = len(buf) - i
// 		}
// 		out, err := rsa.EncryptOAEP(sha, rand.Reader, pubkey, buf[i:i+k], label) // Outputs 384 Bytes
// 		if err != nil {
// 			l.Debugln("Error Encrypting:", err)
// 			return nil, err
// 		}

// 		ret = append(ret, out...)

// 		offset += len(out)
// 	}

// 	return ret, nil
// }

// func Decrypt(buf []byte, label []byte, key *rsa.PrivateKey) (ret []byte, err error) {
// 	// now to encrypting
// 	// each encrypted chunk may only be ((pubkey.N.BitLen() + 7) / 8) - 11 byte big, so we may have to cut her

// 	k := 384

// 	out := make([]byte, 384)
// 	sha := sha256.New()
	
// 	for i := 0; i < len(buf); i += k {
// 		if i + k > len(buf) {
// 			k = len(buf) - i
// 		}
// 		out, err = rsa.DecryptOAEP(sha, rand.Reader, key, buf[i:i+k], label) // Outputs 384 Bytes
// 		if err != nil {
// 			l.Debugln("Error decrypting:", err)
// 			return nil, err
// 		}

// 		ret = append(ret, out...)
// 	}

// 	return ret, nil
// }