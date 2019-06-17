// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"time"

	"github.com/syncthing/syncthing/lib/relay/protocol"
	"github.com/syncthing/syncthing/lib/sync"
	"github.com/syncthing/syncthing/lib/util"

	"github.com/thejerf/suture"
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

func NewClient(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation, timeout time.Duration) (RelayClient, error) {
	factory, ok := supportedSchemes[uri.Scheme]
	if !ok {
		return nil, fmt.Errorf("Unsupported scheme: %s", uri.Scheme)
	}

	return factory(uri, certs, invitations, timeout), nil
}

type commonClient struct {
	*util.Service

	invitations              chan protocol.SessionInvitation
	closeInvitationsOnFinish bool

	stop chan struct{}

	mut sync.RWMutex
	err error
}

func newCommonClient(invitations chan protocol.SessionInvitation, serve func(), stopFn func()) *commonClient {
	c := &commonClient{
		invitations: invitations,

		mut: sync.NewRWMutex(),
	}
	newStop := func() {
		close(c.stop)
		if stopFn != nil {
			stopFn()
		}
	}
	c.Service = util.NewService(serve, newStop)
	if c.invitations == nil {
		c.closeInvitationsOnFinish = true
		c.invitations = make(chan protocol.SessionInvitation)
	}
	return c
}

func (c *staticClient) Serve() {
	defer c.cleanup()
	c.Service.Serve()
}

func (c *commonClient) cleanup() {
	c.mut.Lock()
	if c.closeInvitationsOnFinish {
		close(c.invitations)
	}
	c.mut.Unlock()
}

func (c *commonClient) Invitations() chan protocol.SessionInvitation {
	c.mut.RLock()
	defer c.mut.RUnlock()
	return c.invitations
}

func (c *commonClient) setError(err error) {
	c.mut.Lock()
	c.err = err
	c.mut.Unlock()
}
