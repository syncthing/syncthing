// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"buf.build/go/protovalidate"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/syncthing/syncthing/internal/gen/discoproto"
	"github.com/syncthing/syncthing/lib/protocol"
	"google.golang.org/protobuf/proto"
)

func TestDHTRecordNamespace(t *testing.T) {
	id := protocol.NewDeviceID([]byte("test certificate"))
	key := dhtRecordKey(id)
	if want := "/device/" + id.String(); key != want {
		t.Fatalf("record key: got %q, want %q", key, want)
	}
	validators := record.NamespacedValidator{dhtRecordNamespace: dhtRecordValidator{}}
	if _, ok := validators.ValidatorByKey(key).(dhtRecordValidator); !ok {
		t.Fatalf("no discovery validator registered for %q", key)
	}
}

func TestGenerateDHTDiscoveryGolden(t *testing.T) {
	// (Re-)generates a signed envelope for each of the certificate kinds in
	// testdata

	t.Skip("remove this skip to regenerate the golden envelopes")

	for _, dir := range dhtCertificateDirs(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			// Load the certificate
			cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"))
			if err != nil {
				t.Fatal(err)
			}

			// Generate a random payload
			var random [24]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Fatal(err)
			}

			// Sign it
			signature, err := signPayload(cert.PrivateKey.(crypto.Signer), random[:])
			if err != nil {
				t.Fatal(err)
			}

			// Save it
			data, err := proto.Marshal(&discoproto.DHTEnvelope{
				Version:   1,
				Payload:   random[:],
				Signature: signature,
			})
			if err != nil {
				t.Fatal(err)
			}
			data = base64.StdEncoding.AppendEncode(nil, data)

			if err := os.WriteFile(filepath.Join(dir, "dht.pb64"), data, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDHTDiscoveryGolden(t *testing.T) {
	// Load each envelope and the certificate, verify that the signature
	// checks out.

	for _, dir := range dhtCertificateDirs(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			// Load the discovery envelope
			data, err := os.ReadFile(filepath.Join(dir, "dht.pb64"))
			if err != nil {
				t.Fatal(err)
			}
			data, err = base64.StdEncoding.AppendDecode(nil, data)
			if err != nil {
				t.Fatal(err)
			}
			var envelope discoproto.DHTEnvelope
			if err := proto.Unmarshal(data, &envelope); err != nil {
				t.Fatal(err)
			}
			if err := protovalidate.Validate(&envelope); err != nil {
				t.Fatal(err)
			}
			if len(envelope.Payload) != 24 {
				t.Fatalf("payload length: got %d, want 24", len(envelope.Payload))
			}

			// Load the certificate
			certPEM, err := os.ReadFile(filepath.Join(dir, "cert.pem"))
			if err != nil {
				t.Fatal(err)
			}
			block, _ := pem.Decode(certPEM)
			if block == nil {
				t.Fatal("invalid certificate PEM")
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				t.Fatal(err)
			}

			// Check the signature, it should pass
			if err := checkSignature(cert, &envelope); err != nil {
				t.Error(err)
			}

			// Modify the payload, signature should fail
			envelope.Payload = append(envelope.Payload, 42)
			if err := checkSignature(cert, &envelope); err == nil {
				t.Error("signature should fail")
			}
		})
	}
}

func dhtCertificateDirs(t *testing.T) []string {
	t.Helper()
	certs, err := filepath.Glob("testdata/*/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	if len(certs) == 0 {
		t.Fatal("no certificate fixtures found")
	}
	dirs := make([]string, len(certs))
	for i, cert := range certs {
		dirs[i] = filepath.Dir(cert)
	}
	return dirs
}
