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
	"errors"
	"fmt"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	"github.com/syncthing/syncthing/internal/gen/discoproto"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

const (
	RecordLifetime = 2 * time.Hour

	dhtRecordNamespace = "device"
	dhtSignatureDomain = "syncthing-dht-discovery-v1\x00"
)

func dhtRecordKey(id protocol.DeviceID) string {
	return "/" + dhtRecordNamespace + "/" + id.String()
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
		Version:   1,
		Payload:   bs,
		Signature: signature,
	}
	if err := protovalidate.Validate(envelope); err != nil {
		return nil, err
	}
	return proto.Marshal(envelope)
}

// decodeEnvelope returns the DHTPayload contained in the given signed envelope
func decodeEnvelope(key string, value []byte, now time.Time) (*discoproto.DHTPayload, error) {
	// The key should be a valid device ID
	prefix := "/" + dhtRecordNamespace + "/"
	if !strings.HasPrefix(key, prefix) {
		return nil, errors.New("invalid discovery namespace")
	}
	id, err := protocol.DeviceIDFromString(strings.TrimPrefix(key, prefix))
	if err != nil || key != dhtRecordKey(id) {
		return nil, errors.New("invalid discovery key")
	}

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

func (dhtRecordValidator) Validate(key string, value []byte) error {
	_, err := decodeEnvelope(key, value, time.Now())
	return err
}

// Select returns the index of the best (newest) of the given set of records
func (dhtRecordValidator) Select(key string, values [][]byte) (int, error) {
	idx := -1
	var best *discoproto.DHTPayload
	now := time.Now()
	for i, value := range values {
		rec, err := decodeEnvelope(key, value, now)
		if err != nil {
			continue
		}
		if idx == -1 || rec.Sequence > best.Sequence {
			idx, best = i, rec
		}
	}
	if idx == -1 {
		return 0, errors.New("no valid discovery records")
	}
	return idx, nil
}
