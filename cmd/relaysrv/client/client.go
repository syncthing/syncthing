// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/url"
	"time"

	syncthingprotocol "github.com/syncthing/protocol"
	"github.com/syncthing/relaysrv/protocol"
	"github.com/syncthing/syncthing/internal/sync"
)

type ProtocolClient struct {
	URI         *url.URL
	Invitations chan protocol.SessionInvitation

	closeInvitationsOnFinish bool

	config *tls.Config

	timeout time.Duration

	stop    chan struct{}
	stopped chan struct{}

	conn *tls.Conn

	mut       sync.RWMutex
	connected bool
}

func NewProtocolClient(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation) *ProtocolClient {
	closeInvitationsOnFinish := false
	if invitations == nil {
		closeInvitationsOnFinish = true
		invitations = make(chan protocol.SessionInvitation)
	}

	return &ProtocolClient{
		URI:         uri,
		Invitations: invitations,

		closeInvitationsOnFinish: closeInvitationsOnFinish,

		config: configForCerts(certs),

		timeout: time.Minute * 2,

		stop:    make(chan struct{}),
		stopped: make(chan struct{}),

		mut:       sync.NewRWMutex(),
		connected: false,
	}
}

func (c *ProtocolClient) Serve() {
	c.stop = make(chan struct{})
	c.stopped = make(chan struct{})
	defer close(c.stopped)

	if err := c.connect(); err != nil {
		if debug {
			l.Debugln("Relay connect:", err)
		}
		return
	}

	if debug {
		l.Debugln(c, "connected", c.conn.RemoteAddr())
	}

	if err := c.join(); err != nil {
		c.conn.Close()
		l.Infoln("Relay join:", err)
		return
	}

	if err := c.conn.SetDeadline(time.Time{}); err != nil {
		l.Infoln("Relay set deadline:", err)
		return
	}

	if debug {
		l.Debugln(c, "joined", c.conn.RemoteAddr(), "via", c.conn.LocalAddr())
	}

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
			if debug {
				log.Printf("%s received message %T", c, message)
			}

			switch msg := message.(type) {
			case protocol.Ping:
				if err := protocol.WriteMessage(c.conn, protocol.Pong{}); err != nil {
					l.Infoln("Relay write:", err)
					return

				}
				if debug {
					l.Debugln(c, "sent pong")
				}

			case protocol.SessionInvitation:
				ip := net.IP(msg.Address)
				if len(ip) == 0 || ip.IsUnspecified() {
					msg.Address = c.conn.RemoteAddr().(*net.TCPAddr).IP[:]
				}
				c.Invitations <- msg

			default:
				l.Infoln("Relay: protocol error: unexpected message %v", msg)
				return
			}

		case <-c.stop:
			if debug {
				l.Debugln(c, "stopping")
			}
			return

		case err := <-errors:
			l.Infoln("Relay received:", err)
			return

		case <-timeout.C:
			if debug {
				l.Debugln(c, "timed out")
			}
			return
		}
	}
}

func (c *ProtocolClient) Stop() {
	if c.stop == nil {
		return
	}

	close(c.stop)
	<-c.stopped
}

func (c *ProtocolClient) StatusOK() bool {
	c.mut.RLock()
	con := c.connected
	c.mut.RUnlock()
	return con
}

func (c *ProtocolClient) String() string {
	return fmt.Sprintf("ProtocolClient@%p", c)
}

func (c *ProtocolClient) connect() error {
	if c.URI.Scheme != "relay" {
		return fmt.Errorf("Unsupported relay schema:", c.URI.Scheme)
	}

	conn, err := tls.Dial("tcp", c.URI.Host, c.config)
	if err != nil {
		return err
	}

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		conn.Close()
		return err
	}

	if err := performHandshakeAndValidation(conn, c.URI); err != nil {
		conn.Close()
		return err
	}

	c.conn = conn
	return nil
}

func (c *ProtocolClient) cleanup() {
	if c.closeInvitationsOnFinish {
		close(c.Invitations)
		c.Invitations = make(chan protocol.SessionInvitation)
	}

	if debug {
		l.Debugln(c, "cleaning up")
	}

	c.mut.Lock()
	c.connected = false
	c.mut.Unlock()

	c.conn.Close()
}

func (c *ProtocolClient) join() error {
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
