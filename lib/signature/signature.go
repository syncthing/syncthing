// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// Package signature provides simple methods to create and verify signatures
// in PEM format.
package signature

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/syncthing/syncthing/lib/rand"
	"github.com/syncthing/syncthing/lib/sha256"
)

// GenerateKeys returns a new key pair, with the private and public key
// encoded in PEM format.
func GenerateKeys() (privKey []byte, pubKey []byte, err error) {
	// Generate a new key pair
	key, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	// Marshal the private key
	bs, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}

	// Encode it in PEM format
	privKey = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: bs,
	})

	// Marshal the public key
	bs, err = x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode it in PEM format
	pubKey = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PUBLIC KEY",
		Bytes: bs,
	})

	return
}

// Sign computes the hash of data and signs it with the private key, returning
// a signature in PEM format.
func Sign(privKeyPEM []byte, data io.Reader) ([]byte, error) {
	// Parse the private key
	key, err := loadPrivateKey(privKeyPEM)
	if err != nil {
		return nil, err
	}

	// Hash the reader data
	hash, err := hashReader(data)
	if err != nil {
		return nil, err
	}

	// Sign the hash
	r, s, err := ecdsa.Sign(rand.Reader, key, hash)
	if err != nil {
		return nil, err
	}

	// Marshal the signature using ASN.1
	sig, err := marshalSignature(r, s)
	if err != nil {
		return nil, err
	}

	// Encode it in a PEM block
	bs := pem.EncodeToMemory(&pem.Block{
		Type:  "SIGNATURE",
		Bytes: sig,
	})

	return bs, nil
}

// Verify computes the hash of data and compares it to the signature using the
// given public key. Returns nil if the signature is correct.
func Verify(pubKeyPEM []byte, signature []byte, data io.Reader) error {
	// Parse the public key
	key, err := loadPublicKey(pubKeyPEM)
	if err != nil {
		return err
	}

	// Parse the signature
	block, _ := pem.Decode(signature)
	if block == nil || block.Bytes == nil {
		return errors.New("unsupported signature format")
	}

	r, s, err := unmarshalSignature(block.Bytes)
	if err != nil {
		return err
	}

	// Compute the hash of the data
	hash, err := hashReader(data)
	if err != nil {
		return err
	}

	// Verify the signature
	if !ecdsa.Verify(key, hash, r, s) {
		return errors.New("incorrect signature")
	}

	return nil
}

// hashReader returns the SHA256 hash of the reader
func hashReader(r io.Reader) ([]byte, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	hash := []byte(fmt.Sprintf("%x", h.Sum(nil)))
	return hash, nil
}

// loadPrivateKey returns the ECDSA private key structure for the given PEM
// data.
func loadPrivateKey(bs []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(bs)
	return x509.ParseECPrivateKey(block.Bytes)
}

// loadPublicKey returns the ECDSA public key structure for the given PEM
// data.
func loadPublicKey(bs []byte) (*ecdsa.PublicKey, error) {
	// Decode and parse the public key PEM block
	block, _ := pem.Decode(bs)
	if block == nil || block.Bytes == nil {
		return nil, errors.New("unsupported public key format")
	}
	intf, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	// It should be an ECDSA public key
	pk, ok := intf.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("unsupported public key format")
	}

	return pk, nil
}

// A wrapper around the signature integers so that we can marshal and
// unmarshal them.
type signature struct {
	R, S *big.Int
}

// marshalSignature returns ASN.1 encoded bytes for the given integers,
// suitable for PEM encoding.
func marshalSignature(r, s *big.Int) ([]byte, error) {
	sig := signature{
		R: r,
		S: s,
	}

	bs, err := asn1.Marshal(sig)
	if err != nil {
		return nil, err
	}

	return bs, nil
}

// unmarshalSignature returns the R and S integers from the given ASN.1
// encoded signature.
func unmarshalSignature(sig []byte) (r *big.Int, s *big.Int, err error) {
	var ts signature
	_, err = asn1.Unmarshal(sig, &ts)
	if err != nil {
		return nil, nil, err
	}

	return ts.R, ts.S, nil
}
