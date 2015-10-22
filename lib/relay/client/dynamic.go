// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/syncthing/syncthing/lib/osutil"
	"github.com/syncthing/syncthing/lib/relay/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type dynamicClient struct {
	pooladdr                 *url.URL
	certs                    []tls.Certificate
	invitations              chan protocol.SessionInvitation
	closeInvitationsOnFinish bool

	mut    sync.RWMutex
	client RelayClient
	stop   chan struct{}
}

func newDynamicClient(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation) RelayClient {
	closeInvitationsOnFinish := false
	if invitations == nil {
		closeInvitationsOnFinish = true
		invitations = make(chan protocol.SessionInvitation)
	}
	return &dynamicClient{
		pooladdr:                 uri,
		certs:                    certs,
		invitations:              invitations,
		closeInvitationsOnFinish: closeInvitationsOnFinish,

		mut: sync.NewRWMutex(),
	}
}

func (c *dynamicClient) Serve() {
	c.mut.Lock()
	c.stop = make(chan struct{})
	c.mut.Unlock()

	uri := *c.pooladdr

	// Trim off the `dynamic+` prefix
	uri.Scheme = uri.Scheme[8:]

	l.Debugln(c, "looking up dynamic relays")

	data, err := http.Get(uri.String())
	if err != nil {
		l.Debugln(c, "failed to lookup dynamic relays", err)
		return
	}

	var ann dynamicAnnouncement
	err = json.NewDecoder(data.Body).Decode(&ann)
	data.Body.Close()
	if err != nil {
		l.Debugln(c, "failed to lookup dynamic relays", err)
		return
	}

	defer c.cleanup()

	var addrs []string
	for _, relayAnn := range ann.Relays {
		ruri, err := url.Parse(relayAnn.URL)
		if err != nil {
			l.Debugln(c, "failed to parse dynamic relay address", relayAnn.URL, err)
			continue
		}
		l.Debugln(c, "found", ruri)
		addrs = append(addrs, ruri.String())
	}

	for _, addr := range relayAddressesSortedByLatency(addrs) {
		select {
		case <-c.stop:
			l.Debugln(c, "stopping")
			return
		default:
			ruri, err := url.Parse(addr)
			if err != nil {
				l.Debugln(c, "skipping relay", addr, err)
				continue
			}
			client, err := NewClient(ruri, c.certs, c.invitations)
			if err != nil {
				continue
			}
			c.mut.Lock()
			c.client = client
			c.mut.Unlock()

			c.client.Serve()

			c.mut.Lock()
			c.client = nil
			c.mut.Unlock()
		}
	}
	l.Debugln(c, "could not find a connectable relay")
}

func (c *dynamicClient) Stop() {
	c.mut.RLock()
	defer c.mut.RUnlock()
	close(c.stop)
	if c.client == nil {
		return
	}
	c.client.Stop()
}

func (c *dynamicClient) StatusOK() bool {
	c.mut.RLock()
	defer c.mut.RUnlock()
	if c.client == nil {
		return false
	}
	return c.client.StatusOK()
}

func (c *dynamicClient) Latency() time.Duration {
	c.mut.RLock()
	defer c.mut.RUnlock()
	if c.client == nil {
		return time.Hour
	}
	return c.client.Latency()
}

func (c *dynamicClient) String() string {
	return fmt.Sprintf("DynamicClient:%p:%s@%s", c, c.URI(), c.pooladdr)
}

func (c *dynamicClient) URI() *url.URL {
	c.mut.RLock()
	defer c.mut.RUnlock()
	if c.client == nil {
		return c.pooladdr
	}
	return c.client.URI()
}

func (c *dynamicClient) Invitations() chan protocol.SessionInvitation {
	c.mut.RLock()
	inv := c.invitations
	c.mut.RUnlock()
	return inv
}

func (c *dynamicClient) cleanup() {
	c.mut.Lock()
	if c.closeInvitationsOnFinish {
		close(c.invitations)
		c.invitations = make(chan protocol.SessionInvitation)
	}
	c.mut.Unlock()
}

// This is the announcement recieved from the relay server;
// {"relays": [{"url": "relay://10.20.30.40:5060"}, ...]}
type dynamicAnnouncement struct {
	Relays []struct {
		URL string
	}
}

// relayAddressesSortedByLatency adds local latency to the relay, and sorts them
// by sum latency, and returns the addresses.
func relayAddressesSortedByLatency(input []string) []string {
	relays := make(relayList, len(input))
	for i, relay := range input {
		if latency, err := osutil.GetLatencyForURL(relay); err == nil {
			relays[i] = relayWithLatency{relay, int(latency / time.Millisecond)}
		} else {
			relays[i] = relayWithLatency{relay, int(time.Hour / time.Millisecond)}
		}
	}

	sort.Sort(relays)

	addresses := make([]string, len(relays))
	for i, relay := range relays {
		addresses[i] = relay.relay
	}
	return addresses
}

type relayWithLatency struct {
	relay   string
	latency int
}

type relayList []relayWithLatency

func (l relayList) Len() int {
	return len(l)
}

func (l relayList) Less(a, b int) bool {
	return l[a].latency < l[b].latency
}

func (l relayList) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}
