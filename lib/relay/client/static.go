// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/pkg/errors"

	"github.com/syncthing/syncthing/lib/dialer"
	syncthingprotocol "github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/relay/protocol"
)

type staticClient struct {
	commonClient

	uri *url.URL

	config *tls.Config

	messageTimeout time.Duration
	connectTimeout time.Duration

	conn *tls.Conn

	connected bool
	latency   time.Duration
}

func newStaticClient(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation, timeout time.Duration) RelayClient {
	c := &staticClient{
		uri: uri,

		config: configForCerts(certs),

		messageTimeout: time.Minute * 2,
		connectTimeout: timeout,
	}
	c.commonClient = newCommonClient(invitations, c.serve, c.String())
	return c
}

func (c *staticClient) serve(ctx context.Context) error {
	if err := c.connect(ctx); err != nil {
		l.Debugf("Could not connect to relay %s: %s", c.uri, err)
		return err
	}

	l.Debugln(c, "connected", c.conn.RemoteAddr())
	defer c.disconnect()

	if err := c.join(); err != nil {
		l.Debugf("Could not join relay %s: %s", c.uri, err)
		return err
	}

	if err := c.conn.SetDeadline(time.Time{}); err != nil {
		l.Debugln("Relay set deadline:", err)
		return err
	}

	l.Infof("Joined relay %s://%s", c.uri.Scheme, c.uri.Host)

	c.mut.Lock()
	c.connected = true
	c.mut.Unlock()

	messages := make(chan interface{})
	errorsc := make(chan error, 1)

	go messageReader(ctx, c.conn, messages, errorsc)

	timeout := time.NewTimer(c.messageTimeout)

	for {
		select {
		case message := <-messages:
			timeout.Reset(c.messageTimeout)
			l.Debugf("%s received message %T", c, message)

			switch msg := message.(type) {
			case protocol.Ping:
				if err := protocol.WriteMessage(c.conn, protocol.Pong{}); err != nil {
					l.Debugln("Relay write:", err)
					return err
				}
				l.Debugln(c, "sent pong")

			case protocol.SessionInvitation:
				ip := net.IP(msg.Address)
				if len(ip) == 0 || ip.IsUnspecified() {
					msg.Address = remoteIPBytes(c.conn)
				}
				c.invitations <- msg

			case protocol.RelayFull:
				l.Debugf("Disconnected from relay %s due to it becoming full.", c.uri)
				return errors.New("relay full")

			default:
				l.Debugln("Relay: protocol error: unexpected message %v", msg)
				return fmt.Errorf("protocol error: unexpected message %v", msg)
			}

		case <-ctx.Done():
			l.Debugln(c, "stopping")
			return ctx.Err()

		case err := <-errorsc:
			l.Debugf("Disconnecting from relay %s due to error: %s", c.uri, err)
			return err

		case <-timeout.C:
			l.Debugln(c, "timed out")
			return errors.New("timed out")
		}
	}
}

func (c *staticClient) StatusOK() bool {
	c.mut.RLock()
	con := c.connected
	c.mut.RUnlock()
	return con
}

func (c *staticClient) Latency() time.Duration {
	c.mut.RLock()
	lat := c.latency
	c.mut.RUnlock()
	return lat
}

func (c *staticClient) String() string {
	return fmt.Sprintf("StaticClient:%p@%s", c, c.URI())
}

func (c *staticClient) URI() *url.URL {
	return c.uri
}

func (c *staticClient) connect(ctx context.Context) error {
	if c.uri.Scheme != "relay" {
		return fmt.Errorf("unsupported relay scheme: %v", c.uri.Scheme)
	}

	t0 := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, c.connectTimeout)
	defer cancel()
	tcpConn, err := dialer.DialContext(timeoutCtx, "tcp", c.uri.Host)
	if err != nil {
		return err
	}

	c.mut.Lock()
	c.latency = time.Since(t0)
	c.mut.Unlock()

	conn := tls.Client(tcpConn, c.config)

	if err := conn.SetDeadline(time.Now().Add(c.connectTimeout)); err != nil {
		conn.Close()
		return err
	}

	if err := performHandshakeAndValidation(conn, c.uri); err != nil {
		conn.Close()
		return err
	}

	c.conn = conn
	return nil
}

func (c *staticClient) disconnect() {
	l.Debugln(c, "disconnecting")
	c.mut.Lock()
	c.connected = false
	c.mut.Unlock()

	c.conn.Close()
}

func (c *staticClient) join() error {
	if err := protocol.WriteMessage(c.conn, protocol.JoinRelayRequest{}); err != nil {
		return err
	}

	message, err := protocol.ReadMessage(c.conn)
	if err != nil {
		return err
	}

	switch msg := message.(type) {
	case protocol.Response:
		if msg.Code != 0 {
			return &incorrectResponseCodeErr{msg.Code, msg.Message}
		}

	case protocol.RelayFull:
		return errors.New("relay full")

	default:
		return fmt.Errorf("protocol error: expecting response got %v", msg)
	}

	return nil
}

func performHandshakeAndValidation(conn *tls.Conn, uri *url.URL) error {
	if err := conn.Handshake(); err != nil {
		return err
	}

	cs := conn.ConnectionState()
	if !cs.NegotiatedProtocolIsMutual || cs.NegotiatedProtocol != protocol.ProtocolName {
		return errors.New("protocol negotiation error")
	}

	q := uri.Query()
	relayIDs := q.Get("id")
	if relayIDs != "" {
		relayID, err := syncthingprotocol.DeviceIDFromString(relayIDs)
		if err != nil {
			return errors.Wrap(err, "relay address contains invalid verification id")
		}

		certs := cs.PeerCertificates
		if cl := len(certs); cl != 1 {
			return fmt.Errorf("unexpected certificate count: %d", cl)
		}

		remoteID := syncthingprotocol.NewDeviceID(certs[0].Raw)
		if remoteID != relayID {
			return fmt.Errorf("relay id does not match. Expected %v got %v", relayID, remoteID)
		}
	}

	return nil
}

func messageReader(ctx context.Context, conn net.Conn, messages chan<- interface{}, errors chan<- error) {
	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			errors <- err
			return
		}
		select {
		case messages <- msg:
		case <-ctx.Done():
			return
		}
	}
}
