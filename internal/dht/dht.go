// Copyright (C) 2026 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dht

import (
	"context"
	stdcrypto "crypto"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"time"

	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p"
	kad "github.com/libp2p/go-libp2p-kad-dht"
	record "github.com/libp2p/go-libp2p-record"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pprotocol "github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/quicreuse"
	ma "github.com/multiformats/go-multiaddr"
	quicgo "github.com/quic-go/quic-go"

	"github.com/syncthing/syncthing/internal/gen/discoproto"
	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/rand"
)

const (
	p2pProtocolPrefix libp2pprotocol.ID = "/syncthing"
	p2pProtocolID                       = p2pProtocolPrefix + "/kad/1.0.0"
)

var ErrP2PNotFound = errors.New("peer-to-peer discovery record not found")

// Lookup is a utility function for creating a single-use DHT instance and
// performing a lookup, without starting the full discovery machinery.
func Lookup(ctx context.Context, deviceID protocol.DeviceID, options Options) ([]string, error) {
	dht, err := NewSyncthingDHT(options)
	if err != nil {
		return nil, err
	}
	defer dht.Close()

	for !dht.Connected() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	return dht.Lookup(ctx, deviceID)
}

type Options struct {
	UDP                *registry.UDPReuseManager
	ListenAddresses    []netip.AddrPort
	BootstrapPeers     []string
	AdvertiseAddresses []string
	Identity           []byte
	Server             bool
	ValueDatastore     ds.Batching
	RoutingDatastore   ds.Batching
	MaxConnections     int // Zero uses libp2p defaults.
}

type DHT struct {
	host host.Host
	ipfs *kad.IpfsDHT
}

func NewSyncthingDHT(options Options) (*DHT, error) {
	if options.Server && len(options.ListenAddresses) == 0 {
		return nil, errors.New("DHT server requires a QUIC listener")
	}
	peers := make([]peer.AddrInfo, 0, len(options.BootstrapPeers))
	peerIndices := make(map[peer.ID]int)
	for _, address := range options.BootstrapPeers {
		info, err := peer.AddrInfoFromString(address)
		if err != nil {
			return nil, fmt.Errorf("invalid DHT bootstrap peer: %w", err)
		}
		if index, ok := peerIndices[info.ID]; ok {
			peers[index].Addrs = append(peers[index].Addrs, info.Addrs...)
		} else {
			peerIndices[info.ID] = len(peers)
			peers = append(peers, *info)
		}
	}

	hostOptions := []libp2p.Option{
		libp2p.DisableRelay(),
		libp2p.EnableNATService(),
		libp2p.Transport(quic.NewTransport),
		libp2p.QUICReuse(options.UDP.QUICReuse),
	}

	var listen []ma.Multiaddr
	for _, address := range options.ListenAddresses {
		addr, err := quicreuse.ToQuicMultiaddr(net.UDPAddrFromAddrPort(address), quicgo.Version1)
		if err != nil {
			return nil, err
		}
		listen = append(listen, addr)
	}
	if len(listen) == 0 {
		hostOptions = append(hostOptions, libp2p.NoListenAddrs)
	} else {
		hostOptions = append(hostOptions, libp2p.ListenAddrs(listen...))
	}

	if options.Server {
		hostOptions = append(hostOptions, libp2p.ForceReachabilityPublic())
	}

	if options.Identity != nil {
		key, err := p2pcrypto.UnmarshalPrivateKey(options.Identity)
		if err != nil {
			return nil, err
		}
		hostOptions = append(hostOptions, libp2p.Identity(key))
	}

	if len(options.AdvertiseAddresses) == 0 {
		// For each listen address, try to look up external addresses (port
		// mappings, STUN, ...) and add those to the set of advertised
		// listen addresses.
		hostOptions = append(hostOptions, libp2p.AddrsFactory(func(addresses []ma.Multiaddr) []ma.Multiaddr {
			addresses = slices.Clone(addresses)
			for _, address := range options.UDP.ExternalAddresses(options.ListenAddresses...) {
				if mapped, err := quicreuse.ToQuicMultiaddr(net.UDPAddrFromAddrPort(address), quicgo.Version1); err == nil {
					addresses = append(addresses, mapped)
				}
			}
			return ma.Unique(addresses)
		}))
	} else {
		// Use the defined set of advertised addresses
		var advertised []ma.Multiaddr
		for _, address := range options.AdvertiseAddresses {
			a, err := ma.NewMultiaddr(address)
			if err != nil {
				return nil, err
			}
			advertised = append(advertised, a)
		}
		hostOptions = append(hostOptions, libp2p.AddrsFactory(func([]ma.Multiaddr) []ma.Multiaddr {
			return slices.Clone(advertised)
		}))
	}

	var connections *connmgr.BasicConnMgr
	var resources network.ResourceManager
	if limit := options.MaxConnections; limit > 0 {
		var err error
		// Prune at 90% down to 80%, leaving headroom below the hard limit.
		connections, err = connmgr.NewConnManager(limit-limit/5, limit-limit/10)
		if err != nil {
			return nil, err
		}

		defaults := rcmgr.DefaultLimits
		libp2p.SetDefaultServiceLimits(&defaults)

		limits := rcmgr.PartialLimitConfig{
			System: rcmgr.ResourceLimits{
				Conns:         rcmgr.LimitVal(limit),
				ConnsInbound:  rcmgr.LimitVal(limit),
				ConnsOutbound: rcmgr.LimitVal(limit),
			},
		}.Build(defaults.AutoScale())

		resources, err = rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limits))
		if err != nil {
			connections.Close()
			return nil, err
		}
		hostOptions = append(hostOptions, libp2p.ConnectionManager(connections), libp2p.ResourceManager(resources))
	}

	h, err := libp2p.New(hostOptions...)
	if err != nil {
		if connections != nil {
			connections.Close()
			resources.Close()
		}
		return nil, err
	}

	mode := kad.ModeAuto
	if options.Server {
		mode = kad.ModeServer
	}

	opts := []kad.Option{
		kad.BootstrapOrder(kad.PersistedFirst),
		kad.BootstrapPeers(peers...),
		kad.DisableProviders(),
		kad.MaxRecordAge(RecordLifetime),
		kad.Mode(mode),
		kad.ProtocolPrefix(p2pProtocolPrefix),
		kad.RoutingTablePersistence(5 * time.Minute),
		kad.Validator(record.NamespacedValidator{dhtRecordNamespace: dhtRecordValidator{}}),
		kad.ValueGCInterval(time.Minute),
	}

	if options.ValueDatastore != nil {
		opts = append(opts, kad.ValueDatastore(options.ValueDatastore))
	} else {
		opts = append(opts, kad.ValueDatastore(dssync.MutexWrap(ds.NewMapDatastore())))
	}

	if options.RoutingDatastore != nil {
		opts = append(opts, kad.Datastore(options.RoutingDatastore))
	} else {
		opts = append(opts, kad.Datastore(dssync.MutexWrap(ds.NewMapDatastore())))
	}

	d, err := kad.New(h, opts...)
	if err != nil {
		h.Close()
		return nil, err
	}

	return &DHT{h, d}, nil
}

func (s *DHT) ID() string {
	return s.host.ID().String()
}

func (s *DHT) Connected() bool {
	return s.ipfs.RoutingTable().Size() > 0
}

func (s *DHT) Mode() string {
	// The query handler is registered only in server mode.
	if slices.Contains(s.host.Mux().Protocols(), p2pProtocolID) {
		return "server"
	}
	return "client"
}

func (s *DHT) Close() error {
	return errors.Join(s.ipfs.Close(), s.host.Close())
}

func (s *DHT) Lookup(ctx context.Context, deviceID protocol.DeviceID) ([]string, error) {
	key := dhtRecordKey(deviceID)
	value, err := s.ipfs.GetValue(ctx, key)
	if errors.Is(err, routing.ErrNotFound) {
		slog.DebugContext(ctx, "No DHT result", "device", deviceID, "key", key)
		return nil, ErrP2PNotFound
	}
	if err != nil {
		return nil, err
	}

	r, err := decodeEnvelope(key, value, time.Now())
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "Got DHT result", "device", deviceID, "key", key, "sequence", r.Sequence, "expires", r.ExpiresUnix, "certlen", len(r.Certificate), "addrlen", len(r.Addresses))

	return r.Addresses, nil
}

func (s *DHT) Announce(ctx context.Context, deviceID protocol.DeviceID, addresses []string, certificate []byte, privateKey stdcrypto.Signer) error {
	if s.ipfs.RoutingTable().Size() == 0 {
		return errors.New("no discovery peers")
	}

	key := dhtRecordKey(deviceID)
	var prevSeq int64
	if value, err := s.ipfs.GetValue(ctx, key); err == nil {
		if old, err := decodeEnvelope(key, value, time.Now()); err == nil {
			prevSeq = old.Sequence
		}
	}

	seq := max(prevSeq+1, time.Now().UnixMilli())
	r := &discoproto.DHTPayload{
		Certificate: certificate,
		Addresses:   addresses,
		Sequence:    seq,
		ExpiresUnix: time.Now().Add(RecordLifetime).Unix(),
	}
	value, err := encodeEnvelope(r, privateKey)
	if err != nil {
		return err
	}
	if err := (dhtRecordValidator{}).Validate(key, value); err != nil {
		return err
	}
	if err := s.ipfs.PutValue(ctx, key, value); err != nil {
		return err
	}

	return nil
}

// NewIdentity returns the bytes for a new Ed25519 key usable as
// Options.Identity
func NewIdentity() ([]byte, error) {
	priv, _, err := p2pcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	return p2pcrypto.MarshalPrivateKey(priv)
}
