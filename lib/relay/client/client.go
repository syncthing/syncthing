// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/relay/protocol"
)

type relayClientFactory func(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation, timeout time.Duration) RelayClient

var (
	supportedSchemes = map[string]relayClientFactory{
		"relay":         newStaticClient,
		"dynamic+http":  newDynamicClient,
		"dynamic+https": newDynamicClient,
	}
)

type RelayClient interface {
	Serve()
	Stop()
	Error() error
	Latency() time.Duration
	String() string
	Invitations() chan protocol.SessionInvitation
	URI() *url.URL
}

func NewClient(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation, timeout time.Duration) (RelayClient, error) {
	factory, ok := supportedSchemes[uri.Scheme]
	if !ok {
		return nil, fmt.Errorf("Unsupported scheme: %s", uri.Scheme)
	}

	return factory(uri, certs, invitations, timeout), nil
}
