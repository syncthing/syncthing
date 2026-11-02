// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"context"
	"crypto"
	"crypto/tls"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/syncthing/syncthing/internal/db"
	"github.com/syncthing/syncthing/internal/dht"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

const p2pAddressCheckInterval = time.Minute

type PeerToPeerDiscovery struct {
	errorHolder
	dht         *dht.DHT
	options     dht.Options
	certificate []byte
	privateKey  crypto.Signer
	deviceID    protocol.DeviceID
	addresses   AddressLister
}

var _ FinderService = (*PeerToPeerDiscovery)(nil)

func NewPeerToPeerDiscovery(cert tls.Certificate, addresses AddressLister, kv db.KV, opts dht.Options) (*PeerToPeerDiscovery, error) {
	privateKey, ok := cert.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, errors.New("TLS private key must implement crypto.Signer")
	}

	if opts.ValueDatastore == nil {
		opts.ValueDatastore = dht.NewDatastore(db.NewTyped(kv, "dht-values"))
	}

	if opts.RoutingDatastore == nil {
		opts.RoutingDatastore = dht.NewDatastore(db.NewTyped(kv, "dht-routing"))
	}

	if opts.Identity == nil {
		miscDB := db.NewMiscDB(kv)
		const key = "dht-identity"
		if bs, ok, _ := miscDB.Bytes(key); ok {
			opts.Identity = bs
		} else {
			bs, err := dht.NewIdentity()
			if err != nil {
				return nil, err
			}
			_ = miscDB.PutBytes(key, bs)
			opts.Identity = bs
		}
	}

	dht, err := dht.NewSyncthingDHT(opts)
	if err != nil {
		return nil, err
	}

	s := &PeerToPeerDiscovery{
		dht:         dht,
		options:     opts,
		certificate: slices.Clone(cert.Certificate[0]),
		privateKey:  privateKey,
		deviceID:    protocol.NewDeviceID(cert.Certificate[0]),
		addresses:   addresses,
	}

	s.setError(errors.New("not announced"))
	return s, nil
}

func (s *PeerToPeerDiscovery) String() string {
	return "Peer-to-peer discovery"
}

func (s *PeerToPeerDiscovery) Mode() string {
	return s.dht.Mode()
}

func (s *PeerToPeerDiscovery) Close() error {
	return s.dht.Close()
}

func (s *PeerToPeerDiscovery) Serve(ctx context.Context) error {
	defer s.Close()

	ticker := time.NewTicker(p2pAddressCheckInterval)
	defer ticker.Stop()

	var published []string
	var next time.Time
	for {
		addresses := s.announcementAddresses()

		switch {
		case !s.dht.Connected():
			s.setError(errors.New("no discovery DHT peers"))

		case time.Now().After(next), !slices.Equal(addresses, published):
			err := s.Announce(ctx)
			s.setError(err)
			if err == nil {
				published = addresses
				next = time.Now().Add(dht.RecordLifetime / 2)
			} else {
				next = time.Now().Add(announceErrorRetryInterval)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func announceableAddress(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" {
		return false
	}
	ip, err := netip.ParseAddr(u.Hostname())
	return err != nil || !ip.IsUnspecified()
}

func (s *PeerToPeerDiscovery) announcementAddresses() []string {
	addresses := sanitizeRelayAddresses(slices.Clone(s.addresses.ExternalAddresses()))
	addresses = resolveDHTAddresses(addresses, s.addresses.ExternalIPs())
	addresses = slices.DeleteFunc(addresses, func(address string) bool {
		return !announceableAddress(address)
	})
	slices.Sort(addresses)
	return slices.Compact(addresses)
}

// Resolve wildcard listeners using public IPs obtained directly from STUN.
func resolveDHTAddresses(addresses []string, publicIPs []netip.Addr) []string {
	for _, address := range slices.Clone(addresses) {
		// We want direct-connection addresses with a defined port
		u, err := url.Parse(address)
		if err != nil || !isDirectScheme(u.Scheme) || u.Port() == "" || u.Port() == "0" {
			continue
		}
		// ... where the host part is the unspecified address
		ip, err := netip.ParseAddr(u.Hostname())
		if u.Hostname() != "" && (err != nil || !ip.IsUnspecified()) {
			continue
		}
		// Replace them with addresses using our public IPs but the same port and scheme.
		for _, publicIP := range publicIPs {
			if strings.HasSuffix(u.Scheme, "4") && !publicIP.Is4() || strings.HasSuffix(u.Scheme, "6") && publicIP.Is4() {
				continue
			}
			resolved := *u
			resolved.Host = net.JoinHostPort(publicIP.String(), u.Port())
			addresses = append(addresses, resolved.String())
		}
	}
	return addresses
}

func isDirectScheme(scheme string) bool {
	switch scheme {
	case "tcp", "tcp4", "tcp6", "quic", "quic4", "quic6":
		return true
	default:
		return false
	}
}

func (s *PeerToPeerDiscovery) Announce(ctx context.Context) error {
	return s.dht.Announce(ctx, s.deviceID, s.announcementAddresses(), s.certificate, s.privateKey)
}

func (s *PeerToPeerDiscovery) Lookup(ctx context.Context, deviceID protocol.DeviceID) ([]string, error) {
	return s.dht.Lookup(ctx, deviceID)
}

func (s *PeerToPeerDiscovery) Cache() map[protocol.DeviceID]CacheEntry {
	// no internal caching
	return nil
}

func p2pQUICListenAddresses(addresses []string) []netip.AddrPort {
	var result []netip.AddrPort
	for _, address := range addresses {
		u, err := url.Parse(address)
		if err != nil || (u.Scheme != "quic" && u.Scheme != "quic4" && u.Scheme != "quic6") {
			continue
		}
		port := u.Port()
		if port == "" {
			port = strconv.Itoa(config.DefaultQUICPort)
		}
		network := "udp"
		if u.Scheme == "quic4" {
			network = "udp4"
		}
		if u.Scheme == "quic6" {
			network = "udp6"
		}
		addr, err := net.ResolveUDPAddr(network, net.JoinHostPort(u.Hostname(), port))
		if err != nil || addr.Port == 0 {
			continue
		}
		if addr.IP == nil {
			addr.IP = net.IPv4zero
			if network == "udp6" {
				addr.IP = net.IPv6zero
			}
		}
		if network == "udp" && addr.IP.IsUnspecified() {
			result = append(result, netip.AddrPortFrom(netip.IPv6Unspecified(), uint16(addr.Port)))
			addr.IP = net.IPv4zero
		}
		local := addr.AddrPort()
		result = append(result, netip.AddrPortFrom(local.Addr().Unmap(), local.Port()))
	}
	return result
}
