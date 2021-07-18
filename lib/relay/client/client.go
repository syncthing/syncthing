// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/relay/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/sync"

	"github.com/thejerf/suture/v4"
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
	suture.Service
	Error() error
	Latency() time.Duration
	String() string
	Invitations() chan protocol.SessionInvitation
	URI() *url.URL
}

func NewClient(uri *url.URL, certs []tls.Certificate, timeout time.Duration) (RelayClient, error) {
	factory, ok := supportedSchemes[uri.Scheme]
	if !ok {
		return nil, fmt.Errorf("unsupported scheme: %s", uri.Scheme)
	}

	invitations := make(chan protocol.SessionInvitation)
	return factory(uri, certs, invitations, timeout), nil
}

type commonClient struct {
	svcutil.ServiceWithError

	invitations chan protocol.SessionInvitation
	mut         sync.RWMutex
}

func newCommonClient(invitations chan protocol.SessionInvitation, serve func(context.Context) error, creator string) commonClient {
	c := commonClient{
		invitations: invitations,
		mut:         sync.NewRWMutex(),
	}
	c.ServiceWithError = svcutil.AsService(serve, creator)
	return c
}

func (c *commonClient) Invitations() chan protocol.SessionInvitation {
	c.mut.RLock()
	defer c.mut.RUnlock()
	return c.invitations
}
