// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	"github.com/syncthing/syncthing/internal/gen/discoproto"
	"github.com/syncthing/syncthing/lib/protocol"
	"golang.org/x/crypto/chacha20poly1305"
	"google.golang.org/protobuf/proto"
)

const (
	RecordLifetime = 2 * time.Hour

	dhtRecordNamespace = "device"
	dhtSignatureDomain = "syncthing-dht-discovery-v1\x00"
)

// dhtRecordKey encodes the public half of the daily record signing key.
func dhtRecordKey(public ed25519.PublicKey) string {
	return "/" + dhtRecordNamespace + "/" + base64.RawURLEncoding.EncodeToString(public)
}

// encodeEnvelope returns a signed DHTEnvelope containing the payload
func encodeEnvelope(payload *discoproto.DHTPayload, key crypto.Signer) ([]byte, error) {
	if err := protovalidate.Validate(payload); err != nil {
		return nil, err
	}
	bs, err := proto.Marshal(payload)
	if err != nil {
		return nil, err
	}
	signature, err := signPayload(key, bs)
	if err != nil {
		return nil, err
	}
	envelope := &discoproto.DHTEnvelope{
		Payload:   bs,
		Signature: signature,
	}
	if err := protovalidate.Validate(envelope); err != nil {
		return nil, err
	}
	return proto.Marshal(envelope)
}

// decodeEnvelope returns the DHTPayload contained in the given signed envelope
func decodeEnvelope(id protocol.DeviceID, value []byte, now time.Time) (*discoproto.DHTPayload, error) {
	// We can unmarshal the envelope
	var envelope discoproto.DHTEnvelope
	if err := proto.Unmarshal(value, &envelope); err != nil {
		return nil, err
	}
	if err := protovalidate.Validate(&envelope); err != nil {
		return nil, err
	}

	// We can unmarshal the message inside
	var payload discoproto.DHTPayload
	if err := proto.Unmarshal(envelope.Payload, &payload); err != nil {
		return nil, err
	}
	if err := protovalidate.Validate(&payload); err != nil {
		return nil, err
	}

	// We can parse the certificate inside the message
	cert, err := x509.ParseCertificate(payload.Certificate)
	if err != nil {
		return nil, fmt.Errorf("bad certificate: %w", err)
	}
	// ... and verify that it signed the envelope
	if err := checkSignature(cert, &envelope); err != nil {
		return nil, fmt.Errorf("invalid discovery signature: %w", err)
	}

	// It should not have expired. We also check the expiry is not in the
	// far future, while keeping some leeway for increasing the expiry time
	// in the future.
	expires := time.Unix(payload.ExpiresUnix, 0)
	if now.After(expires) || expires.After(now.Add(4*RecordLifetime)) {
		return nil, errors.New("invalid discovery expiration")
	}

	// The certificate device ID should match the key we asked for
	calculatedID := protocol.NewDeviceID(payload.Certificate)
	if id != calculatedID {
		return nil, errors.New("incorrect device ID")
	}

	return &payload, nil
}

// checkSignature verifies that the payload in the envelope was signed by the certificate
func checkSignature(cert *x509.Certificate, envelope *discoproto.DHTEnvelope) error {
	algorithm, err := dhtSignatureAlgorithm(cert.PublicKey)
	if err != nil {
		return err
	}
	if err := cert.CheckSignature(algorithm, append([]byte(dhtSignatureDomain), envelope.Payload...), envelope.Signature); err != nil {
		return fmt.Errorf("invalid discovery signature: %w", err)
	}
	return nil
}

// signPayload creates the signature for the payload, in the format expected by checkSignature
func signPayload(key crypto.Signer, payload []byte) ([]byte, error) {
	algorithm, err := dhtSignatureAlgorithm(key.Public())
	if err != nil {
		return nil, err
	}
	message := append([]byte(dhtSignatureDomain), payload...)
	var opts crypto.SignerOpts = crypto.Hash(0)
	if algorithm != x509.PureEd25519 {
		digest := sha256.Sum256(message)
		message = digest[:]
		opts = crypto.SHA256
	}
	if algorithm == x509.SHA256WithRSAPSS {
		opts = &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}
	}
	return key.Sign(rand.Reader, message, opts)
}

func dhtSignatureAlgorithm(public crypto.PublicKey) (x509.SignatureAlgorithm, error) {
	switch public.(type) {
	case ed25519.PublicKey:
		return x509.PureEd25519, nil
	case *ecdsa.PublicKey:
		return x509.ECDSAWithSHA256, nil
	case *rsa.PublicKey:
		return x509.SHA256WithRSAPSS, nil
	default:
		return x509.UnknownSignatureAlgorithm, fmt.Errorf("unsupported discovery public key type %T", public)
	}
}

// dhtRecordValidator is used by libp2p to validate records when receiving
// them from the network
type dhtRecordValidator struct{}

const encryptedRecordDomain = "syncthing-dht-encrypted-record-v1\x00"

// wrapperMetadata is also used as AEAD additional data, binding encryption
// to the wrapper version, sequence and expiration.
func wrapperMetadata(w *discoproto.DHTWrapper) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(&discoproto.DHTWrapper{
		Version:     w.Version,
		Sequence:    w.Sequence,
		ExpiresUnix: w.ExpiresUnix,
	})
}

func wrapperSigningBytes(w *discoproto.DHTWrapper) ([]byte, error) {
	unsigned := proto.Clone(w).(*discoproto.DHTWrapper)
	unsigned.Signature = nil
	bs, err := (proto.MarshalOptions{Deterministic: true}).Marshal(unsigned)
	if err != nil {
		return nil, err
	}
	return append([]byte(encryptedRecordDomain), bs...), nil
}

func encryptRecord(payload *discoproto.DHTPayload, signer crypto.Signer, keys discoveryKeys) ([]byte, error) {
	value, err := encodeEnvelope(payload, signer)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(keys.encryption[:])
	if err != nil {
		return nil, err
	}
	w := &discoproto.DHTWrapper{
		Version:     1,
		Sequence:    payload.Sequence,
		ExpiresUnix: payload.ExpiresUnix,
	}
	additional, err := wrapperMetadata(w)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	w.Payload = aead.Seal(nonce, nonce, value, additional)
	signed, err := wrapperSigningBytes(w)
	if err != nil {
		return nil, err
	}
	w.Signature = ed25519.Sign(keys.signing, signed)
	if err := protovalidate.Validate(w); err != nil {
		return nil, err
	}
	return proto.Marshal(w)
}

func validateEncryptedRecord(key string, value []byte, now time.Time) (*discoproto.DHTWrapper, error) {
	prefix := "/" + dhtRecordNamespace + "/"
	if !strings.HasPrefix(key, prefix) {
		return nil, errors.New("invalid discovery namespace")
	}
	public, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(key, prefix))
	if err != nil || len(public) != ed25519.PublicKeySize || key != dhtRecordKey(public) {
		return nil, errors.New("invalid discovery key")
	}
	if len(value) > 8192 {
		return nil, errors.New("invalid encrypted discovery record size")
	}
	var w discoproto.DHTWrapper
	if err := proto.Unmarshal(value, &w); err != nil {
		return nil, err
	}
	if err := protovalidate.Validate(&w); err != nil {
		return nil, err
	}
	signed, err := wrapperSigningBytes(&w)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(public, signed, w.Signature) {
		return nil, errors.New("invalid encrypted discovery signature")
	}
	expires := time.Unix(w.ExpiresUnix, 0)
	if now.After(expires) || expires.After(now.Add(4*RecordLifetime)) {
		return nil, errors.New("invalid encrypted discovery expiration")
	}
	return &w, nil
}

func decryptRecord(id protocol.DeviceID, value []byte, keys discoveryKeys, now time.Time) (*discoproto.DHTPayload, error) {
	w, err := validateEncryptedRecord(keys.record, value, now)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(keys.encryption[:])
	if err != nil {
		return nil, err
	}
	additional, err := wrapperMetadata(w)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, w.Payload[:aead.NonceSize()], w.Payload[aead.NonceSize():], additional)
	if err != nil {
		return nil, err
	}
	payload, err := decodeEnvelope(id, plaintext, now)
	if err != nil {
		return nil, err
	}
	if payload.Sequence != w.Sequence || payload.ExpiresUnix != w.ExpiresUnix {
		return nil, errors.New("discovery metadata mismatch")
	}
	return payload, nil
}

func (dhtRecordValidator) Validate(key string, value []byte) error {
	_, err := validateEncryptedRecord(key, value, time.Now())
	return err
}

// Select returns the index of the newest valid encrypted record.
func (dhtRecordValidator) Select(key string, values [][]byte) (int, error) {
	idx := -1
	var best int64
	now := time.Now()
	for i, value := range values {
		w, err := validateEncryptedRecord(key, value, now)
		if err == nil && (idx == -1 || w.Sequence > best) {
			idx, best = i, w.Sequence
		}
	}
	if idx == -1 {
		return 0, errors.New("no valid discovery records")
	}
	return idx, nil
}
