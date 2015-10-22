// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"time"

	syncthingprotocol "github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/relay/protocol"
	"github.com/syncthing/syncthing/lib/sync"
)

type staticClient struct {
	uri         *url.URL
	invitations chan protocol.SessionInvitation

	closeInvitationsOnFinish bool

	config *tls.Config

	timeout time.Duration

	stop    chan struct{}
	stopped chan struct{}

	conn *tls.Conn

	mut       sync.RWMutex
	connected bool
	latency   time.Duration
}

func newStaticClient(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation) RelayClient {
	closeInvitationsOnFinish := false
	if invitations == nil {
		closeInvitationsOnFinish = true
		invitations = make(chan protocol.SessionInvitation)
	}

	return &staticClient{
		uri:         uri,
		invitations: invitations,

		closeInvitationsOnFinish: closeInvitationsOnFinish,

		config: configForCerts(certs),

		timeout: time.Minute * 2,

		stop:    make(chan struct{}),
		stopped: make(chan struct{}),

		mut:       sync.NewRWMutex(),
		connected: false,
	}
}

func (c *staticClient) Serve() {
	c.stop = make(chan struct{})
	c.stopped = make(chan struct{})
	defer close(c.stopped)

	if err := c.connect(); err != nil {
		l.Debugln("Relay connect:", err)
		return
	}

	l.Debugln(c, "connected", c.conn.RemoteAddr())

	if err := c.join(); err != nil {
		c.conn.Close()
		l.Infoln("Relay join:", err)
		return
	}

	if err := c.conn.SetDeadline(time.Time{}); err != nil {
		c.conn.Close()
		l.Infoln("Relay set deadline:", err)
		return
	}

	l.Debugln(c, "joined", c.conn.RemoteAddr(), "via", c.conn.LocalAddr())

	defer c.cleanup()
	c.mut.Lock()
	c.connected = true
	c.mut.Unlock()

	messages := make(chan interface{})
	errors := make(chan error, 1)

	go messageReader(c.conn, messages, errors)

	timeout := time.NewTimer(c.timeout)

	for {
		select {
		case message := <-messages:
			timeout.Reset(c.timeout)
			l.Debugf("%s received message %T", c, message)

			switch msg := message.(type) {
			case protocol.Ping:
				if err := protocol.WriteMessage(c.conn, protocol.Pong{}); err != nil {
					l.Infoln("Relay write:", err)
					return

				}
				l.Debugln(c, "sent pong")

			case protocol.SessionInvitation:
				ip := net.IP(msg.Address)
				if len(ip) == 0 || ip.IsUnspecified() {
					msg.Address = c.conn.RemoteAddr().(*net.TCPAddr).IP[:]
				}
				c.invitations <- msg

			default:
				l.Infoln("Relay: protocol error: unexpected message %v", msg)
				return
			}

		case <-c.stop:
			l.Debugln(c, "stopping")
			return

		case err := <-errors:
			l.Infoln("Relay received:", err)
			return

		case <-timeout.C:
			l.Debugln(c, "timed out")
			return
		}
	}
}

func (c *staticClient) Stop() {
	if c.stop == nil {
		return
	}

	close(c.stop)
	<-c.stopped
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

func (c *staticClient) Invitations() chan protocol.SessionInvitation {
	c.mut.RLock()
	inv := c.invitations
	c.mut.RUnlock()
	return inv
}

func (c *staticClient) connect() error {
	if c.uri.Scheme != "relay" {
		return fmt.Errorf("Unsupported relay schema: %v", c.uri.Scheme)
	}

	t0 := time.Now()
	tcpConn, err := net.Dial("tcp", c.uri.Host)
	if err != nil {
		return err
	}

	c.mut.Lock()
	c.latency = time.Since(t0)
	c.mut.Unlock()

	conn := tls.Client(tcpConn, c.config)
	if err = conn.Handshake(); err != nil {
		return err
	}

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
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

func (c *staticClient) cleanup() {
	l.Debugln(c, "cleaning up")
	c.mut.Lock()
	if c.closeInvitationsOnFinish {
		close(c.invitations)
		c.invitations = make(chan protocol.SessionInvitation)
	}
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
			return fmt.Errorf("Incorrect response code %d: %s", msg.Code, msg.Message)
		}

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
		return fmt.Errorf("protocol negotiation error")
	}

	q := uri.Query()
	relayIDs := q.Get("id")
	if relayIDs != "" {
		relayID, err := syncthingprotocol.DeviceIDFromString(relayIDs)
		if err != nil {
			return fmt.Errorf("relay address contains invalid verification id: %s", err)
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

func messageReader(conn net.Conn, messages chan<- interface{}, errors chan<- error) {
	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			errors <- err
			return
		}
		messages <- msg
	}
}
