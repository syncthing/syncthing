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
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"buf.build/go/protovalidate"
	"github.com/syncthing/syncthing/internal/gen/discoproto"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

const (
	RecordLifetime = 2 * time.Hour

	dhtSignatureDomain = "syncthing-dht-discovery-v1\x00"
	dhtRecordKeySeed   = "syncthing-dht-record-key-v1\x00"
	dhtRecordNamespace = "device"
)

// discoveryTimeValue staggers daily rotation across devices.
func discoveryTimeValue(id protocol.DeviceID, now time.Time) uint64 {
	return (uint64(now.Unix()) + binary.BigEndian.Uint64(id[:8])) / 86400 //nolint:gosec // intentionally wraps
}

// dhtRecordKey hashes the seed, raw device ID and big-endian time value.
func dhtRecordKey(id protocol.DeviceID, timeValue uint64) string {
	var input [len(dhtRecordKeySeed) + len(protocol.DeviceID{}) + 8]byte
	offset := copy(input[:], dhtRecordKeySeed)
	offset += copy(input[offset:], id[:])
	binary.BigEndian.PutUint64(input[offset:], timeValue)
	digest := sha256.Sum256(input[:])
	return "/" + dhtRecordNamespace + "/" + base64.RawURLEncoding.EncodeToString(digest[:])
}

// discoveryTimeValues includes the previous period while an announcement
// from that period can still be valid.
func discoveryTimeValues(id protocol.DeviceID, now time.Time) []uint64 {
	current := discoveryTimeValue(id, now)
	oldest := discoveryTimeValue(id, now.Add(-RecordLifetime))
	if oldest != current {
		return []uint64{current, oldest}
	}
	return []uint64{current}
}

// encodeRecord returns a signed DHTRecord containing the payload
func encodeRecord(payload *discoproto.DHTPayload, key crypto.Signer) ([]byte, error) {
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
	record := &discoproto.DHTRecord{
		Payload:   bs,
		Signature: signature,
	}
	if err := protovalidate.Validate(record); err != nil {
		return nil, err
	}
	return proto.Marshal(record)
}

// validateRecord verifies the signed record and binds it to its DHT key.
func validateRecord(key string, value []byte, now time.Time) (*discoproto.DHTPayload, error) {
	if len(value) > 8192 {
		return nil, errors.New("invalid discovery record size")
	}
	// We can unmarshal the record
	var record discoproto.DHTRecord
	if err := proto.Unmarshal(value, &record); err != nil {
		return nil, err
	}
	if err := protovalidate.Validate(&record); err != nil {
		return nil, err
	}

	// We can unmarshal the message inside
	var payload discoproto.DHTPayload
	if err := proto.Unmarshal(record.Payload, &payload); err != nil {
		return nil, err
	}
	if err := protovalidate.Validate(&payload); err != nil {
		return nil, err
	}

	id := protocol.NewDeviceID(payload.Certificate)
	if key != dhtRecordKey(id, payload.TimeValue) {
		return nil, errors.New("incorrect discovery record key")
	}

	// We can parse the certificate inside the message
	cert, err := x509.ParseCertificate(payload.Certificate)
	if err != nil {
		return nil, fmt.Errorf("bad certificate: %w", err)
	}
	// ... and verify that it signed the record
	if err := checkSignature(cert, &record); err != nil {
		return nil, fmt.Errorf("invalid discovery signature: %w", err)
	}

	// It should not have expired. We also check the expiry is not in the
	// far future, while keeping some leeway for increasing the expiry time
	// in the future.
	if payload.ExpiresUnix > math.MaxInt64 {
		return nil, errors.New("invalid discovery expiration")
	}
	expires := time.Unix(int64(payload.ExpiresUnix), 0)
	if now.After(expires) || expires.After(now.Add(4*RecordLifetime)) {
		return nil, errors.New("invalid discovery expiration")
	}

	return &payload, nil
}

// checkSignature verifies that the payload in the record was signed by the certificate
func checkSignature(cert *x509.Certificate, record *discoproto.DHTRecord) error {
	algorithm, err := dhtSignatureAlgorithm(cert.PublicKey)
	if err != nil {
		return err
	}
	if err := cert.CheckSignature(algorithm, append([]byte(dhtSignatureDomain), record.Payload...), record.Signature); err != nil {
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
	_, err := validateRecord(key, value, time.Now())
	return err
}

// Select returns the index of the newest valid record.
func (dhtRecordValidator) Select(key string, values [][]byte) (int, error) {
	idx := -1
	var best uint64
	now := time.Now()
	for i, value := range values {
		payload, err := validateRecord(key, value, now)
		if err == nil && (idx == -1 || payload.ExpiresUnix > best) {
			idx, best = i, payload.ExpiresUnix
		}
	}
	if idx == -1 {
		return 0, errors.New("no valid discovery records")
	}
	return idx, nil
}
