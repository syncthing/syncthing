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

	"github.com/thejerf/suture/v4"
)

type RelayClient interface {
	suture.Service
	Error() error
	String() string
	Invitations() <-chan protocol.SessionInvitation
	URI() *url.URL
}

func NewClient(uri *url.URL, certs []tls.Certificate, timeout time.Duration) (RelayClient, error) {
	invitations := make(chan protocol.SessionInvitation)

	switch uri.Scheme {
	case "relay":
		return newStaticClient(uri, certs, invitations, timeout), nil
	case "dynamic+http", "dynamic+https":
		return newDynamicClient(uri, certs, invitations, timeout), nil
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", uri.Scheme)
	}
}

type commonClient struct {
	svcutil.ServiceWithError

	invitations chan protocol.SessionInvitation
}

func newCommonClient(invitations chan protocol.SessionInvitation, serve func(context.Context) error, creator string) commonClient {
	return commonClient{
		ServiceWithError: svcutil.AsService(serve, creator),
		invitations:      invitations,
	}
}

func (c *commonClient) Invitations() <-chan protocol.SessionInvitation {
	return c.invitations
}
