// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package connections

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/tlsutil"
)

func TestFixupPort(t *testing.T) {
	cases := [][2]string{
		{"tcp://1.2.3.4:5", "tcp://1.2.3.4:5"},
		{"tcp://1.2.3.4:", "tcp://1.2.3.4:22000"},
		{"tcp://1.2.3.4", "tcp://1.2.3.4:22000"},
		{"tcp://[fe80::1]", "tcp://[fe80::1]:22000"},
		{"tcp://[fe80::1]:", "tcp://[fe80::1]:22000"},
		{"tcp://[fe80::1]:22000", "tcp://[fe80::1]:22000"},
		{"tcp://[fe80::1]:22000", "tcp://[fe80::1]:22000"},
		{"tcp://[fe80::1%25abc]", "tcp://[fe80::1%25abc]:22000"},
		{"tcp://[fe80::1%25abc]:", "tcp://[fe80::1%25abc]:22000"},
		{"tcp://[fe80::1%25abc]:22000", "tcp://[fe80::1%25abc]:22000"},
		{"tcp://[fe80::1%25abc]:22000", "tcp://[fe80::1%25abc]:22000"},
	}

	for _, tc := range cases {
		u0, err := url.Parse(tc[0])
		if err != nil {
			t.Fatal(err)
		}
		u1 := fixupPort(u0, 22000).String()
		if u1 != tc[1] {
			t.Errorf("fixupPort(%q, 22000) => %q, expected %q", tc[0], u1, tc[1])
		}
	}
}

func TestAllowedNetworks(t *testing.T) {
	cases := []struct {
		host    string
		allowed []string
		ok      bool
	}{
		{
			"192.168.0.1",
			nil,
			false,
		},
		{
			"192.168.0.1",
			[]string{},
			false,
		},
		{
			"fe80::1",
			nil,
			false,
		},
		{
			"fe80::1",
			[]string{},
			false,
		},
		{
			"192.168.0.1",
			[]string{"fe80::/48", "192.168.0.0/24"},
			true,
		},
		{
			"fe80::1",
			[]string{"192.168.0.0/24", "fe80::/48"},
			true,
		},
		{
			"192.168.0.1",
			[]string{"192.168.1.0/24", "fe80::/48"},
			false,
		},
		{
			"fe80::1",
			[]string{"fe82::/48", "192.168.1.0/24"},
			false,
		},
		{
			"192.168.0.1:4242",
			[]string{"fe80::/48", "192.168.0.0/24"},
			true,
		},
		{
			"[fe80::1]:4242",
			[]string{"192.168.0.0/24", "fe80::/48"},
			true,
		},
		{
			"10.20.30.40",
			[]string{"!10.20.30.0/24", "10.0.0.0/8"},
			false,
		},
		{
			"10.20.30.40",
			[]string{"10.0.0.0/8", "!10.20.30.0/24"},
			true,
		},
		{
			"[fe80::1]:4242",
			[]string{"192.168.0.0/24", "!fe00::/8", "fe80::/48"},
			false,
		},
	}

	for _, tc := range cases {
		res := IsAllowedNetwork(tc.host, tc.allowed)
		if res != tc.ok {
			t.Errorf("allowedNetwork(%q, %q) == %v, want %v", tc.host, tc.allowed, res, tc.ok)
		}
	}
}

func TestGetDialer(t *testing.T) {
	mustParseURI := func(v string) *url.URL {
		uri, err := url.Parse(v)
		if err != nil {
			panic(err)
		}
		return uri
	}

	cases := []struct {
		uri        *url.URL
		ok         bool
		disabled   bool
		deprecated bool
	}{
		{mustParseURI("tcp://1.2.3.4:5678"), true, false, false},   // ok
		{mustParseURI("tcp4://1.2.3.4:5678"), true, false, false},  // ok
		{mustParseURI("kcp://1.2.3.4:5678"), false, false, true},   // deprecated
		{mustParseURI("relay://1.2.3.4:5678"), false, true, false}, // disabled
		{mustParseURI("http://1.2.3.4:5678"), false, false, false}, // generally bad
		{mustParseURI("bananas!"), false, false, false},            // wat
	}

	cfg := config.New(protocol.LocalDeviceID)
	cfg.Options.RelaysEnabled = false

	for _, tc := range cases {
		df, err := getDialerFactory(cfg, tc.uri)
		if tc.ok && err != nil {
			t.Errorf("getDialerFactory(%q) => %v, expected nil err", tc.uri, err)
		}
		if tc.ok && df == nil {
			t.Errorf("getDialerFactory(%q) => nil factory, expected non-nil", tc.uri)
		}
		if tc.deprecated && err != errDeprecated {
			t.Errorf("getDialerFactory(%q) => %v, expected %v", tc.uri, err, errDeprecated)
		}
		if tc.disabled && err != errDisabled {
			t.Errorf("getDialerFactory(%q) => %v, expected %v", tc.uri, err, errDisabled)
		}

		lf, err := getListenerFactory(cfg, tc.uri)
		if tc.ok && err != nil {
			t.Errorf("getListenerFactory(%q) => %v, expected nil err", tc.uri, err)
		}
		if tc.ok && lf == nil {
			t.Errorf("getListenerFactory(%q) => nil factory, expected non-nil", tc.uri)
		}
		if tc.deprecated && err != errDeprecated {
			t.Errorf("getListenerFactory(%q) => %v, expected %v", tc.uri, err, errDeprecated)
		}
		if tc.disabled && err != errDisabled {
			t.Errorf("getListenerFactory(%q) => %v, expected %v", tc.uri, err, errDisabled)
		}
	}
}

func TestConnectionStatus(t *testing.T) {
	s := newConnectionStatusHandler()

	addr := "testAddr"
	testErr := errors.New("testErr")

	if stats := s.ConnectionStatus(); len(stats) != 0 {
		t.Fatal("newly created connectionStatusHandler isn't empty:", len(stats))
	}

	check := func(in, out error) {
		t.Helper()
		s.setConnectionStatus(addr, in)
		switch stat, ok := s.ConnectionStatus()[addr]; {
		case !ok:
			t.Fatal("entry missing")
		case out == nil:
			if stat.Error != nil {
				t.Fatal("expected nil error, got", stat.Error)
			}
		case *stat.Error != out.Error():
			t.Fatalf("expected %v error, got %v", out.Error(), *stat.Error)
		}
	}

	check(nil, nil)

	check(context.Canceled, nil)

	check(testErr, testErr)

	check(context.Canceled, testErr)

	check(nil, nil)
}

func TestNextDialRegistryCleanup(t *testing.T) {
	now := time.Now()
	firsts := []time.Time{
		now.Add(-dialCoolDownInterval + time.Second),
		now.Add(-dialCoolDownDelay + time.Second),
		now.Add(-2 * dialCoolDownDelay),
	}

	r := make(nextDialRegistry)

	// Cases where the device should be cleaned up

	r[protocol.LocalDeviceID] = nextDialDevice{}
	r.sleepDurationAndCleanup(now)
	if l := len(r); l > 0 {
		t.Errorf("Expected empty to be cleaned up, got length %v", l)
	}
	for _, dev := range []nextDialDevice{
		// attempts below threshold, outside of interval
		{
			attempts:              1,
			coolDownIntervalStart: firsts[1],
		},
		{
			attempts:              1,
			coolDownIntervalStart: firsts[2],
		},
		// Threshold reached, but outside of cooldown delay
		{
			attempts:              dialCoolDownMaxAttempts,
			coolDownIntervalStart: firsts[2],
		},
	} {
		r[protocol.LocalDeviceID] = dev
		r.sleepDurationAndCleanup(now)
		if l := len(r); l > 0 {
			t.Errorf("attempts: %v, start: %v: Expected all cleaned up, got length %v", dev.attempts, dev.coolDownIntervalStart, l)
		}
	}

	// Cases where the device should stay monitored
	for _, dev := range []nextDialDevice{
		// attempts below threshold, inside of interval
		{
			attempts:              1,
			coolDownIntervalStart: firsts[0],
		},
		// attempts at threshold, inside delay
		{
			attempts:              dialCoolDownMaxAttempts,
			coolDownIntervalStart: firsts[0],
		},
		{
			attempts:              dialCoolDownMaxAttempts,
			coolDownIntervalStart: firsts[1],
		},
	} {
		r[protocol.LocalDeviceID] = dev
		r.sleepDurationAndCleanup(now)
		if l := len(r); l != 1 {
			t.Errorf("attempts: %v, start: %v: Expected device still tracked, got length %v", dev.attempts, dev.coolDownIntervalStart, l)
		}
	}
}

func BenchmarkConnections(b *testing.B) {
	addrs := []string{
		"tcp://127.0.0.1:0",
		"quic://127.0.0.1:0",
		"relay://127.0.0.1:22067",
	}
	sizes := []int{
		1 << 10,
		1 << 15,
		1 << 20,
		1 << 22,
	}
	haveRelay := false
	// Check if we have a relay running locally
	conn, err := net.DialTimeout("tcp", "127.0.0.1:22067", 100*time.Millisecond)
	if err == nil {
		haveRelay = true
		_ = conn.Close()
	}
	for _, addr := range addrs {
		for _, sz := range sizes {
			data := make([]byte, sz)
			if _, err := rand.Read(data); err != nil {
				b.Fatal(err)
			}
			for _, direction := range []string{"cs", "sc"} {
				proto := strings.SplitN(addr, ":", 2)[0]
				b.Run(fmt.Sprintf("%s_%d_%s", proto, sz, direction), func(b *testing.B) {
					if proto == "relay" && !haveRelay {
						b.Skip("could not connect to relay")
					}
					withConnectionPair(b, addr, func(client, server internalConn) {
						if direction == "sc" {
							server, client = client, server
						}

						total := 0
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var wg sync.WaitGroup
							wg.Add(2)
							errC := make(chan error, 2)
							go func() {
								if _, err := client.Write(data); err != nil {
									errC <- err
									return
								}
								wg.Done()
							}()
							go func() {
								if _, err := io.ReadFull(server, data); err != nil {
									errC <- err
									return
								}
								total += sz
								wg.Done()
							}()
							wg.Wait()
							close(errC)
							err := <-errC
							if err != nil {
								b.Fatal(err)
							}
						}
						b.ReportAllocs()
						b.SetBytes(int64(total / b.N))
					})
				})
			}
		}
	}
}

func TestConnectionEstablishment(t *testing.T) {
	addrs := []string{
		"tcp://127.0.0.1:0",
		"quic://127.0.0.1:0",
	}

	send := make([]byte, 128<<10)
	if _, err := rand.Read(send); err != nil {
		t.Fatal(err)
	}

	for _, addr := range addrs {
		proto := strings.SplitN(addr, ":", 2)[0]

		t.Run(proto, func(t *testing.T) {
			withConnectionPair(t, addr, func(client, server internalConn) {
				if _, err := client.Write(send); err != nil {
					t.Fatal(err)
				}

				recv := make([]byte, len(send))
				if _, err := io.ReadFull(server, recv); err != nil {
					t.Fatal(err)
				}

				if !bytes.Equal(recv, send) {
					t.Fatal("data mismatch")
				}
			})
		})

	}
}

func withConnectionPair(b interface{ Fatal(...interface{}) }, connUri string, h func(client, server internalConn)) {
	// Root of the service tree.
	supervisor := suture.New("main", suture.Spec{
		PassThroughPanics: true,
	})

	cert := mustGetCert(b)
	deviceId := protocol.NewDeviceID(cert.Certificate[0])
	tlsCfg := tlsutil.SecureDefaultTLS13()
	tlsCfg.Certificates = []tls.Certificate{cert}
	tlsCfg.NextProtos = []string{"bench"}
	tlsCfg.ClientAuth = tls.RequestClientCert
	tlsCfg.SessionTicketsDisabled = true
	tlsCfg.InsecureSkipVerify = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	supervisor.ServeBackground(ctx)

	cfg := config.Configuration{
		Options: config.OptionsConfiguration{
			RelaysEnabled: true,
		},
	}
	wcfg := config.Wrap("", cfg, deviceId, events.NoopLogger)
	uri, err := url.Parse(connUri)
	if err != nil {
		b.Fatal(err)
	}
	lf, err := getListenerFactory(cfg, uri)
	if err != nil {
		b.Fatal(err)
	}
	natSvc := nat.NewService(deviceId, wcfg)
	conns := make(chan internalConn, 1)
	lanChecker := &lanChecker{wcfg}
	listenSvc := lf.New(uri, wcfg, tlsCfg, conns, natSvc, registry.New(), lanChecker)
	supervisor.Add(listenSvc)

	var addr *url.URL
	for {
		addrs := listenSvc.LANAddresses()
		if len(addrs) > 0 {
			if !strings.HasSuffix(addrs[0].Host, ":0") {
				addr = addrs[0]
				break
			}
		}
		time.Sleep(time.Millisecond)
	}

	df, err := getDialerFactory(cfg, addr)
	if err != nil {
		b.Fatal(err)
	}
	// Purposely using a different registry: Don't want to reuse port between dialer and listener on the same device
	dialer := df.New(cfg.Options, tlsCfg, registry.New(), lanChecker)

	// Relays might take some time to register the device, so dial multiple times
	clientConn, err := dialer.Dial(ctx, deviceId, addr)
	if err != nil {
		for i := 0; i < 10 && err != nil; i++ {
			clientConn, err = dialer.Dial(ctx, deviceId, addr)
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			b.Fatal(err)
		}
	}

	// Quic does not start a stream until some data is sent through, so send something for the AcceptStream
	// to fire on the other side.
	send := []byte("hello")
	if _, err := clientConn.Write(send); err != nil {
		b.Fatal(err)
	}

	serverConn := <-conns

	recv := make([]byte, len(send))
	if _, err := io.ReadFull(serverConn, recv); err != nil {
		b.Fatal(err)
	}
	if !bytes.Equal(recv, send) {
		b.Fatal("data mismatch")
	}

	h(clientConn, serverConn)

	_ = clientConn.Close()
	_ = serverConn.Close()
}

func mustGetCert(b interface{ Fatal(...interface{}) }) tls.Certificate {
	cert, err := tlsutil.NewCertificateInMemory("bench", 10)
	if err != nil {
		b.Fatal(err)
	}
	return cert
}
