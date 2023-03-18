// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package discover

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	stdsync "sync"
	"time"

	"github.com/syncthing/syncthing/lib/connections/registry"
	"github.com/syncthing/syncthing/lib/dialer"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/protocol"
)

type globalClient struct {
	server         string
	addrList       AddressLister
	announceClient httpClient
	queryClient    httpClient
	noAnnounce     bool
	noLookup       bool
	evLogger       events.Logger
	errorHolder
}

type httpClient interface {
	Get(ctx context.Context, url string) (*http.Response, error)
	Post(ctx context.Context, url, ctype string, data io.Reader) (*http.Response, error)
}

const (
	defaultReannounceInterval             = 30 * time.Minute
	announceErrorRetryInterval            = 5 * time.Minute
	requestTimeout                        = 30 * time.Second
	maxAddressChangesBetweenAnnouncements = 10
)

type announcement struct {
	Addresses []string `json:"addresses"`
}

func (a announcement) MarshalJSON() ([]byte, error) {
	type announcementCopy announcement

	a.Addresses = sanitizeRelayAddresses(a.Addresses)

	aCopy := announcementCopy(a)
	return json.Marshal(aCopy)
}

type serverOptions struct {
	insecure   bool   // don't check certificate
	noAnnounce bool   // don't announce
	noLookup   bool   // don't use for lookups
	id         string // expected server device ID
}

// A lookupError is any other error but with a cache validity time attached.
type lookupError struct {
	msg      string
	cacheFor time.Duration
}

func (e *lookupError) Error() string { return e.msg }

func (e *lookupError) CacheFor() time.Duration {
	return e.cacheFor
}

func NewGlobal(server string, cert tls.Certificate, addrList AddressLister, evLogger events.Logger, registry *registry.Registry) (FinderService, error) {
	server, opts, err := parseOptions(server)
	if err != nil {
		return nil, err
	}

	var devID protocol.DeviceID
	if opts.id != "" {
		devID, err = protocol.DeviceIDFromString(opts.id)
		if err != nil {
			return nil, err
		}
	}

	// The http.Client used for announcements. It needs to have our
	// certificate to prove our identity, and may or may not verify the server
	// certificate depending on the insecure setting.
	var dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	if registry != nil {
		dialContext = dialer.DialContextReusePortFunc(registry)
	} else {
		dialContext = dialer.DialContext
	}
	var announceClient httpClient = &contextClient{&http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			DialContext: dialContext,
			Proxy:       http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: opts.insecure,
				Certificates:       []tls.Certificate{cert},
			},
		},
	}}
	if opts.id != "" {
		announceClient = newIDCheckingHTTPClient(announceClient, devID)
	}

	// The http.Client used for queries. We don't need to present our
	// certificate here, so lets not include it. May be insecure if requested.
	var queryClient httpClient = &contextClient{&http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
			Proxy:       http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: opts.insecure,
			},
		},
	}}
	if opts.id != "" {
		queryClient = newIDCheckingHTTPClient(queryClient, devID)
	}

	cl := &globalClient{
		server:         server,
		addrList:       addrList,
		announceClient: announceClient,
		queryClient:    queryClient,
		noAnnounce:     opts.noAnnounce,
		noLookup:       opts.noLookup,
		evLogger:       evLogger,
	}
	if !opts.noAnnounce {
		// If we are supposed to announce, it's an error until we've done so.
		cl.setError(errors.New("not announced"))
	}

	return cl, nil
}

// Lookup returns the list of addresses where the given device is available
func (c *globalClient) Lookup(ctx context.Context, device protocol.DeviceID) (addresses []string, err error) {
	if c.noLookup {
		return nil, &lookupError{
			msg:      "lookups not supported",
			cacheFor: time.Hour,
		}
	}

	qURL, err := url.Parse(c.server)
	if err != nil {
		return nil, err
	}

	q := qURL.Query()
	q.Set("device", device.String())
	qURL.RawQuery = q.Encode()

	resp, err := c.queryClient.Get(ctx, qURL.String())
	if err != nil {
		l.Debugln("globalClient.Lookup", qURL, err)
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		l.Debugln("globalClient.Lookup", qURL, resp.Status)
		err := errors.New(resp.Status)
		if secs, atoiErr := strconv.Atoi(resp.Header.Get("Retry-After")); atoiErr == nil && secs > 0 {
			err = &lookupError{
				msg:      resp.Status,
				cacheFor: time.Duration(secs) * time.Second,
			}
		}
		return nil, err
	}

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	var ann announcement
	err = json.Unmarshal(bs, &ann)
	return ann.Addresses, err
}

func (c *globalClient) String() string {
	return "global@" + c.server
}

func (c *globalClient) Serve(ctx context.Context) error {
	if c.noAnnounce {
		// We're configured to not do announcements, only lookups. To maintain
		// the same interface, we just pause here if Serve() is run.
		<-ctx.Done()
		return ctx.Err()
	}

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	eventSub := c.evLogger.Subscribe(events.ListenAddressesChanged)
	defer eventSub.Unsubscribe()

	timerResetCount := 0

	for {
		select {
		case <-eventSub.C():
			if timerResetCount < maxAddressChangesBetweenAnnouncements {
				// Defer announcement by 2 seconds, essentially debouncing
				// if we have a stream of events incoming in quick succession.
				timer.Reset(2 * time.Second)
			} else if timerResetCount == maxAddressChangesBetweenAnnouncements {
				// Yet only do it if we haven't had to reset maxAddressChangesBetweenAnnouncements times in a row,
				// so if something is flip-flopping within 2 seconds, we don't end up in a permanent reset loop.
				l.Warnf("Detected a flip-flopping listener")
				c.setError(errors.New("flip flopping listener"))
				// Incrementing the count above 10 will prevent us from warning or setting the error again
				// It will also suppress event based resets until we've had a proper round after announceErrorRetryInterval
				timer.Reset(announceErrorRetryInterval)
			}
			timerResetCount++
		case <-timer.C:
			timerResetCount = 0
			c.sendAnnouncement(ctx, timer)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *globalClient) sendAnnouncement(ctx context.Context, timer *time.Timer) {
	var ann announcement
	if c.addrList != nil {
		ann.Addresses = c.addrList.ExternalAddresses()
	}

	if len(ann.Addresses) == 0 {
		// There are legitimate cases for not having anything to announce,
		// yet still using global discovery for lookups. Do not error out
		// here.
		c.setError(nil)
		timer.Reset(announceErrorRetryInterval)
		return
	}

	// The marshal doesn't fail, I promise.
	postData, _ := json.Marshal(ann)

	l.Debugf("%s Announcement: %v", c, ann)

	resp, err := c.announceClient.Post(ctx, c.server, "application/json", bytes.NewReader(postData))
	if err != nil {
		l.Debugln(c, "announce POST:", err)
		c.setError(err)
		timer.Reset(announceErrorRetryInterval)
		return
	}
	l.Debugln(c, "announce POST:", resp.Status)
	resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		l.Debugln(c, "announce POST:", resp.Status)
		c.setError(errors.New(resp.Status))

		if h := resp.Header.Get("Retry-After"); h != "" {
			// The server has a recommendation on when we should
			// retry. Follow it.
			if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
				l.Debugln(c, "announce Retry-After:", secs, err)
				timer.Reset(time.Duration(secs) * time.Second)
				return
			}
		}

		timer.Reset(announceErrorRetryInterval)
		return
	}

	c.setError(nil)

	if h := resp.Header.Get("Reannounce-After"); h != "" {
		// The server has a recommendation on when we should
		// reannounce. Follow it.
		if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
			l.Debugln(c, "announce Reannounce-After:", secs, err)
			timer.Reset(time.Duration(secs) * time.Second)
			return
		}
	}

	timer.Reset(defaultReannounceInterval)
}

func (*globalClient) Cache() map[protocol.DeviceID]CacheEntry {
	// The globalClient doesn't do caching
	return nil
}

// parseOptions parses and strips away any ?query=val options, setting the
// corresponding field in the serverOptions struct. Unknown query options are
// ignored and removed.
func parseOptions(dsn string) (server string, opts serverOptions, err error) {
	p, err := url.Parse(dsn)
	if err != nil {
		return "", serverOptions{}, err
	}

	// Grab known options from the query string
	q := p.Query()
	opts.id = q.Get("id")
	opts.insecure = opts.id != "" || queryBool(q, "insecure")
	opts.noAnnounce = queryBool(q, "noannounce")
	opts.noLookup = queryBool(q, "nolookup")

	// Check for disallowed combinations
	if p.Scheme == "http" {
		if !opts.insecure {
			return "", serverOptions{}, errors.New("http without insecure not supported")
		}
		if !opts.noAnnounce {
			return "", serverOptions{}, errors.New("http without noannounce not supported")
		}
	} else if p.Scheme != "https" {
		return "", serverOptions{}, errors.New("unsupported scheme " + p.Scheme)
	}

	// Remove the query string
	p.RawQuery = ""
	server = p.String()

	return
}

// queryBool returns the query parameter parsed as a boolean. An empty value
// ("?foo") is considered true, as is any value string except false
// ("?foo=false").
func queryBool(q url.Values, key string) bool {
	if _, ok := q[key]; !ok {
		return false
	}

	return q.Get(key) != "false"
}

type idCheckingHTTPClient struct {
	httpClient
	id protocol.DeviceID
}

func newIDCheckingHTTPClient(client httpClient, id protocol.DeviceID) *idCheckingHTTPClient {
	return &idCheckingHTTPClient{
		httpClient: client,
		id:         id,
	}
}

func (c *idCheckingHTTPClient) check(resp *http.Response) error {
	if resp.TLS == nil {
		return errors.New("security: not TLS")
	}

	if len(resp.TLS.PeerCertificates) == 0 {
		return errors.New("security: no certificates")
	}

	id := protocol.NewDeviceID(resp.TLS.PeerCertificates[0].Raw)
	if !id.Equals(c.id) {
		return errors.New("security: incorrect device id")
	}

	return nil
}

func (c *idCheckingHTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	resp, err := c.httpClient.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := c.check(resp); err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *idCheckingHTTPClient) Post(ctx context.Context, url, ctype string, data io.Reader) (*http.Response, error) {
	resp, err := c.httpClient.Post(ctx, url, ctype, data)
	if err != nil {
		return nil, err
	}
	if err := c.check(resp); err != nil {
		return nil, err
	}

	return resp, nil
}

type errorHolder struct {
	err error
	mut stdsync.Mutex // uses stdlib sync as I want this to be trivially embeddable, and there is no risk of blocking
}

func (e *errorHolder) setError(err error) {
	e.mut.Lock()
	e.err = err
	e.mut.Unlock()
}

func (e *errorHolder) Error() error {
	e.mut.Lock()
	err := e.err
	e.mut.Unlock()
	return err
}

type contextClient struct {
	*http.Client
}

func (c *contextClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *contextClient) Post(ctx context.Context, url, ctype string, data io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, data)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", ctype)
	return c.Client.Do(req)
}

func globalDiscoveryIdentity(addr string) string {
	return "global discovery server " + addr
}

func ipv4Identity(port int) string {
	return fmt.Sprintf("IPv4 local broadcast discovery on port %d", port)
}

func ipv6Identity(addr string) string {
	return fmt.Sprintf("IPv6 local multicast discovery on address %s", addr)
}
