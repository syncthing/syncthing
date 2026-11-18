// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"buf.build/go/protovalidate"
	record "github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/syncthing/syncthing/internal/gen/discoproto"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
	"google.golang.org/protobuf/proto"
)

func TestDiscoveryKeyDerivation(t *testing.T) {
	id := protocol.NewDeviceID([]byte("test certificate"))
	const want = "/device/MdF2NZYnXR_5p58usA2yMMSdb-mKauM9unqhFMXEj-Y"
	if got := dhtRecordKey(id, 42); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestDiscoveryTimeValues(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct {
		name     string
		offset   uint64
		previous uint64
		current  uint64
	}{
		{"daily cutoff", 57_600, 20833, 20834},
		{"uint64 wrap", math.MaxUint64 - 1_800_000_000 + 1, math.MaxUint64 / 86400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var id protocol.DeviceID
			binary.BigEndian.PutUint64(id[:8], tc.offset)
			if got := discoveryTimeValue(id, now.Add(-time.Second)); got != tc.previous {
				t.Errorf("before cutoff: got %d, want %d", got, tc.previous)
			}
			if got := discoveryTimeValues(id, now); !slices.Equal(got, []uint64{tc.current, tc.previous}) {
				t.Errorf("at cutoff: got %v, want [%d %d]", got, tc.current, tc.previous)
			}
			if got := discoveryTimeValues(id, now.Add(RecordLifetime)); !slices.Equal(got, []uint64{tc.current}) {
				t.Errorf("after previous record expires: got %v, want [%d]", got, tc.current)
			}
		})
	}
}

func TestDHTRecordNamespace(t *testing.T) {
	id := protocol.NewDeviceID([]byte("test certificate"))
	key := dhtRecordKey(id, discoveryTimeValue(id, time.Now()))
	if key == "/device/"+id.String() {
		t.Fatal("record key reveals device ID")
	}
	validators := record.NamespacedValidator{dhtRecordNamespace: dhtRecordValidator{}}
	if _, ok := validators.ValidatorByKey(key).(dhtRecordValidator); !ok {
		t.Fatalf("no discovery validator registered for %q", key)
	}
}

func TestDHTDiscoveryGolden(t *testing.T) {
	// Load each record and the certificate, verify that the signature
	// checks out.

	for _, dir := range dhtCertificateDirs(t) {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			// Load the discovery record
			data, err := os.ReadFile(filepath.Join(dir, "dht.pb64"))
			if err != nil {
				t.Fatal(err)
			}
			data, err = base64.StdEncoding.AppendDecode(nil, data)
			if err != nil {
				t.Fatal(err)
			}
			var record discoproto.DHTRecord
			if err := proto.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			if err := protovalidate.Validate(&record); err != nil {
				t.Fatal(err)
			}
			if len(record.Payload) != 24 {
				t.Fatalf("payload length: got %d, want 24", len(record.Payload))
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
			if err := checkSignature(cert, &record); err != nil {
				t.Error(err)
			}

			// Modify the payload, signature should fail
			record.Payload = append(record.Payload, 42)
			if err := checkSignature(cert, &record); err == nil {
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

func TestAnnounceLookup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	// Create a new DHT instance
	newDHT := func(options Options) *DHT {
		t.Helper()
		options.UDP = registry.NewUDPReuseManager()
		t.Cleanup(func() { options.UDP.QUICReuse().Close() })
		instance, err := NewSyncthingDHT(options)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { instance.Close() })
		return instance
	}

	// The server is the bootstrap node in this scenario
	server := newDHT(Options{
		ListenAddresses: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")},
		Server:          true,
	})

	// Connects a client to the bootstrap node
	connect := func(client *DHT) {
		t.Helper()

		// Add a connection to the bootstrap node
		if err := client.host.Connect(ctx, peer.AddrInfo{
			ID:    server.host.ID(),
			Addrs: server.host.Addrs(),
		}); err != nil {
			t.Fatal(err)
		}

		// Wait for the client to consider itself connected
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for !client.Connected() {
			select {
			case <-ctx.Done():
				t.Fatalf("waiting for discovery peer: %v", ctx.Err())
			case <-ticker.C:
			}
		}
	}

	// Create an announcing node, and announce a generated device ID
	announcer := newDHT(Options{})
	connect(announcer)
	cert, err := tlsutil.NewCertificateInMemory("discovery-test", 1)
	if err != nil {
		t.Fatal(err)
	}
	deviceID := protocol.NewDeviceID(cert.Certificate[0])
	addresses := []string{"quic://192.0.2.1:22000", "tcp://192.0.2.1:22000"}
	if err := announcer.Announce(ctx, deviceID, addresses, cert.Certificate[0], cert.PrivateKey.(crypto.Signer)); err != nil {
		t.Fatalf("announcing: %v", err)
	}

	// Create a lookup node, and verify we can look up the announcement from above
	reader := newDHT(Options{})
	connect(reader)
	got, err := reader.Lookup(ctx, deviceID)
	if err != nil {
		t.Fatalf("looking up announcement: %v", err)
	}
	if !slices.Equal(got, addresses) {
		t.Fatalf("lookup returned %v, want %v", got, addresses)
	}
}

func TestRegenerateDHTDiscoveryGolden(t *testing.T) {
	// (Re-)generates a signed record for each of the certificate kinds in
	// testdata

	t.Skip("remove this skip to regenerate the golden records")

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
			data, err := proto.Marshal(&discoproto.DHTRecord{
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

func TestSignedRecord(t *testing.T) {
	cert, err := tlsutil.NewCertificateInMemory("discovery-test", 1)
	if err != nil {
		t.Fatal(err)
	}

	id := protocol.NewDeviceID(cert.Certificate[0])
	now := time.Now().Truncate(time.Second)
	payload := &discoproto.DHTPayload{
		Certificate: cert.Certificate[0],
		Addresses:   []string{"tcp://192.0.2.1:22000"},
		ExpiresUnix: uint64(now.Add(RecordLifetime).Unix()),
		TimeValue:   discoveryTimeValue(id, now),
	}
	key := dhtRecordKey(id, payload.TimeValue)
	signer := cert.PrivateKey.(crypto.Signer)
	value, err := encodeRecord(payload, signer)
	if err != nil {
		t.Fatal(err)
	}

	var record discoproto.DHTRecord
	if err := proto.Unmarshal(value, &record); err != nil {
		t.Fatal(err)
	}
	var decoded discoproto.DHTPayload
	if err := proto.Unmarshal(record.Payload, &decoded); err != nil || !proto.Equal(&decoded, payload) {
		t.Fatalf("decoding plaintext payload: %v, %v", &decoded, err)
	}
	if got, err := validateRecord(key, value, now); err != nil || !proto.Equal(got, payload) {
		t.Fatalf("validating record: %v, %v", got, err)
	}

	for _, wrongKey := range []string{
		dhtRecordKey(id, payload.TimeValue+1),
		dhtRecordKey(protocol.NewDeviceID([]byte("other")), payload.TimeValue),
	} {
		if _, err := validateRecord(wrongKey, value, now); err == nil {
			t.Errorf("accepted record under incorrect key %q", wrongKey)
		}
	}
	if _, err := validateRecord(key, value, now.Add(RecordLifetime+time.Second)); err == nil {
		t.Error("accepted expired record")
	}

	decoded.ExpiresUnix++
	record.Payload, err = proto.Marshal(&decoded)
	if err != nil {
		t.Fatal(err)
	}
	tampered, err := proto.Marshal(&record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateRecord(key, tampered, now); err == nil {
		t.Error("accepted tampered payload")
	}

	newer, err := encodeRecord(&decoded, signer)
	if err != nil {
		t.Fatal(err)
	}
	if idx, err := (dhtRecordValidator{}).Select(key, [][]byte{value, newer, tampered}); err != nil || idx != 1 {
		t.Fatalf("selection: %d, %v", idx, err)
	}
}
