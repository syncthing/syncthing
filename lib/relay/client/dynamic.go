// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
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
	timeout                  time.Duration

	mut    sync.RWMutex
	err    error
	client RelayClient
	stop   chan struct{}
}

func newDynamicClient(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation, timeout time.Duration) RelayClient {
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
		timeout:                  timeout,

		mut: sync.NewRWMutex(),
	}
}

func (c *dynamicClient) Serve() {
	defer c.cleanup()
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
		c.setError(err)
		return
	}

	var ann dynamicAnnouncement
	err = json.NewDecoder(data.Body).Decode(&ann)
	data.Body.Close()
	if err != nil {
		l.Debugln(c, "failed to lookup dynamic relays", err)
		c.setError(err)
		return
	}

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

	for _, addr := range relayAddressesOrder(addrs) {
		select {
		case <-c.stop:
			l.Debugln(c, "stopping")
			c.setError(nil)
			return
		default:
			ruri, err := url.Parse(addr)
			if err != nil {
				l.Debugln(c, "skipping relay", addr, err)
				continue
			}
			client, err := NewClient(ruri, c.certs, c.invitations, c.timeout)
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
	c.setError(fmt.Errorf("could not find a connectable relay"))
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

func (c *dynamicClient) Error() error {
	c.mut.RLock()
	defer c.mut.RUnlock()
	if c.client == nil {
		return c.err
	}
	return c.client.Error()
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
		return nil
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

func (c *dynamicClient) setError(err error) {
	c.mut.Lock()
	c.err = err
	c.mut.Unlock()
}

// This is the announcement received from the relay server;
// {"relays": [{"url": "relay://10.20.30.40:5060"}, ...]}
type dynamicAnnouncement struct {
	Relays []struct {
		URL string
	}
}

// relayAddressesOrder checks the latency to each relay, rounds latency down to
// the closest 50ms, and puts them in buckets of 50ms latency ranges. Then
// shuffles each bucket, and returns all addresses starting with the ones from
// the lowest latency bucket, ending with the highest latency buceket.
func relayAddressesOrder(input []string) []string {
	buckets := make(map[int][]string)

	for _, relay := range input {
		latency, err := osutil.GetLatencyForURL(relay)
		if err != nil {
			latency = time.Hour
		}

		id := int(latency/time.Millisecond) / 50

		buckets[id] = append(buckets[id], relay)
	}

	var ids []int
	for id, bucket := range buckets {
		shuffle(bucket)
		ids = append(ids, id)
	}

	sort.Ints(ids)

	addresses := make([]string, len(input))

	for _, id := range ids {
		addresses = append(addresses, buckets[id]...)
	}

	return addresses
}

func shuffle(slice []string) {
	for i := len(slice) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		slice[i], slice[j] = slice[j], slice[i]
	}
}
