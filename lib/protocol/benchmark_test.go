// Copyright (C) 2016 The Protocol Authors.

package protocol

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"net"
	"testing"

	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/testutils"
)

func BenchmarkRequestsRawTCP(b *testing.B) {
	// Benchmarks the rate at which we can serve requests over a single,
	// unencrypted TCP channel over the loopback interface.

	// Get a connected TCP pair
	conn0, conn1, err := getTCPConnectionPair()
	if err != nil {
		b.Fatal(err)
	}

	defer conn0.Close()
	defer conn1.Close()

	// Bench it
	benchmarkRequestsConnPair(b, conn0, conn1)
}

func BenchmarkRequestsTLSoTCP(b *testing.B) {
	conn0, conn1, err := getTCPConnectionPair()
	if err != nil {
		b.Fatal(err)
	}
	defer conn0.Close()
	defer conn1.Close()
	benchmarkRequestsTLS(b, conn0, conn1)
}

func benchmarkRequestsTLS(b *testing.B, conn0, conn1 net.Conn) {
	// Benchmarks the rate at which we can serve requests over a single,
	// TLS encrypted channel over the loopback interface.

	// Load a certificate, skipping this benchmark if it doesn't exist
	cert, err := tls.LoadX509KeyPair("../../test/h1/cert.pem", "../../test/h1/key.pem")
	if err != nil {
		b.Skip(err)
		return
	}

	/// TLSify them
	conn0, conn1 = negotiateTLS(cert, conn0, conn1)

	// Bench it
	benchmarkRequestsConnPair(b, conn0, conn1)
}

func benchmarkRequestsConnPair(b *testing.B, conn0, conn1 net.Conn) {
	// Start up Connections on them
	c0 := NewConnection(LocalDeviceID, conn0, conn0, testutils.NoopCloser{}, new(fakeModel), new(mockedConnectionInfo), CompressionMetadata)
	c0.Start()
	c1 := NewConnection(LocalDeviceID, conn1, conn1, testutils.NoopCloser{}, new(fakeModel), new(mockedConnectionInfo), CompressionMetadata)
	c1.Start()

	// Satisfy the assertions in the protocol by sending an initial cluster config
	c0.ClusterConfig(ClusterConfig{})
	c1.ClusterConfig(ClusterConfig{})

	// Report some useful stats and reset the timer for the actual test
	b.ReportAllocs()
	b.SetBytes(128 << 10)
	b.ResetTimer()

	// Request 128 KiB blocks, which will be satisfied by zero copy from the
	// other side (we'll get back a full block of zeroes).
	var buf []byte
	var err error
	for i := 0; i < b.N; i++ {
		// Use c0 and c1 for each alternating request, so we get as much
		// data flowing in both directions.
		if i%2 == 0 {
			buf, err = c0.Request(context.Background(), "folder", "file", i, int64(i), 128<<10, nil, 0, false)
		} else {
			buf, err = c1.Request(context.Background(), "folder", "file", i, int64(i), 128<<10, nil, 0, false)
		}

		if err != nil {
			b.Fatal(err)
		}
		if len(buf) != 128<<10 {
			b.Fatal("Incorrect returned buf length", len(buf), "!=", 128<<10)
		}

		// The fake model is supposed to tag the end of the buffer with the
		// requested offset, so we can verify that we get back data for this
		// block correctly.
		if binary.BigEndian.Uint64(buf[128<<10-8:]) != uint64(i) {
			b.Fatal("Bad data returned")
		}
	}
}

// returns the two endpoints of a TCP connection over lo0
func getTCPConnectionPair() (net.Conn, net.Conn, error) {
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}

	// We run the Accept in the background since it's blocking, and we use
	// the channel to make the race thingies happy about writing vs reading
	// conn0 and err0.
	var conn0 net.Conn
	var err0 error
	done := make(chan struct{})
	go func() {
		conn0, err0 = lst.Accept()
		close(done)
	}()

	// Dial the connection
	conn1, err := net.Dial("tcp", lst.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	// Check any error from accept
	<-done
	if err0 != nil {
		return nil, nil, err0
	}

	// Set the buffer sizes etc as usual
	dialer.SetTCPOptions(conn0)
	dialer.SetTCPOptions(conn1)

	return conn0, conn1, nil
}

func negotiateTLS(cert tls.Certificate, conn0, conn1 net.Conn) (net.Conn, net.Conn) {
	cfg := &tls.Config{
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{"bep/1.0"},
		ClientAuth:             tls.RequestClientCert,
		SessionTicketsDisabled: true,
		InsecureSkipVerify:     true,
		MinVersion:             tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		},
	}

	tlsc0 := tls.Server(conn0, cfg)
	tlsc1 := tls.Client(conn1, cfg)
	return tlsc0, tlsc1
}

// The fake model does nothing much

type fakeModel struct{}

func (m *fakeModel) Index(deviceID DeviceID, folder string, files []FileInfo) error {
	return nil
}

func (m *fakeModel) IndexUpdate(deviceID DeviceID, folder string, files []FileInfo) error {
	return nil
}

func (m *fakeModel) Request(deviceID DeviceID, folder, name string, blockNo, size int32, offset int64, hash []byte, weakHash uint32, fromTemporary bool) (RequestResponse, error) {
	// We write the offset to the end of the buffer, so the receiver
	// can verify that it did in fact get some data back over the
	// connection.
	buf := make([]byte, size)
	binary.BigEndian.PutUint64(buf[len(buf)-8:], uint64(offset))
	return &fakeRequestResponse{buf}, nil
}

func (m *fakeModel) ClusterConfig(deviceID DeviceID, config ClusterConfig) error {
	return nil
}

func (m *fakeModel) Closed(conn Connection, err error) {
}

func (m *fakeModel) DownloadProgress(deviceID DeviceID, folder string, updates []FileDownloadProgressUpdate) error {
	return nil
}
