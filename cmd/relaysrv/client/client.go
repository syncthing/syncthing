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
)

func NewProtocolClient(uri *url.URL, certs []tls.Certificate, invitations chan protocol.SessionInvitation) ProtocolClient {
	closeInvitationsOnFinish := false
	if invitations == nil {
		closeInvitationsOnFinish = true
		invitations = make(chan protocol.SessionInvitation)
	}
	return ProtocolClient{
		URI:         uri,
		Invitations: invitations,

		closeInvitationsOnFinish: closeInvitationsOnFinish,

		config: configForCerts(certs),

		timeout: time.Minute * 2,

		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

type ProtocolClient struct {
	URI         *url.URL
	Invitations chan protocol.SessionInvitation

	closeInvitationsOnFinish bool

	config *tls.Config

	timeout time.Duration

	stop    chan struct{}
	stopped chan struct{}

	conn *tls.Conn
}

func (c *ProtocolClient) connect() error {
	conn, err := tls.Dial("tcp", c.URI.Host, c.config)
	if err != nil {
		return err
	}

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	if err := performHandshakeAndValidation(conn, c.URI); err != nil {
		return err
	}

	c.conn = conn
	return nil
}

func (c *ProtocolClient) Serve() {
	if err := c.connect(); err != nil {
		panic(err)
	}

	if debug {
		l.Debugln(c, "connected", c.conn.RemoteAddr())
	}

	if err := c.join(); err != nil {
		c.conn.Close()
		panic(err)
	}

	c.conn.SetDeadline(time.Time{})

	if debug {
		l.Debugln(c, "joined", c.conn.RemoteAddr(), "via", c.conn.LocalAddr())
	}

	c.stop = make(chan struct{})
	c.stopped = make(chan struct{})

	defer c.cleanup()

	messages := make(chan interface{})
	errors := make(chan error, 1)

	go func(conn net.Conn, message chan<- interface{}, errors chan<- error) {
		for {
			msg, err := protocol.ReadMessage(conn)
			if err != nil {
				errors <- err
				return
			}
			messages <- msg
		}
	}(c.conn, messages, errors)

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
					panic(err)
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
				panic(fmt.Errorf("protocol error: unexpected message %v", msg))
			}
		case <-c.stop:
			if debug {
				l.Debugln(c, "stopping")
			}
			break
		case err := <-errors:
			panic(err)
		case <-timeout.C:
			if debug {
				l.Debugln(c, "timed out")
			}
			return
		}
	}

	c.stopped <- struct{}{}
}

func (c *ProtocolClient) Stop() {
	if c.stop == nil {
		return
	}

	c.stop <- struct{}{}
	<-c.stopped
}

func (c *ProtocolClient) String() string {
	return fmt.Sprintf("ProtocolClient@%p", c)
}

func (c *ProtocolClient) cleanup() {
	if c.closeInvitationsOnFinish {
		close(c.Invitations)
		c.Invitations = make(chan protocol.SessionInvitation)
	}

	if debug {
		l.Debugln(c, "cleaning up")
	}

	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}

	if c.stopped != nil {
		close(c.stopped)
		c.stopped = nil
	}

	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *ProtocolClient) join() error {
	err := protocol.WriteMessage(c.conn, protocol.JoinRelayRequest{})
	if err != nil {
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
	err := conn.Handshake()
	if err != nil {
		conn.Close()
		return err
	}

	cs := conn.ConnectionState()
	if !cs.NegotiatedProtocolIsMutual || cs.NegotiatedProtocol != protocol.ProtocolName {
		conn.Close()
		return fmt.Errorf("protocol negotiation error")
	}

	q := uri.Query()
	relayIDs := q.Get("id")
	if relayIDs != "" {
		relayID, err := syncthingprotocol.DeviceIDFromString(relayIDs)
		if err != nil {
			conn.Close()
			return fmt.Errorf("relay address contains invalid verification id: %s", err)
		}

		certs := cs.PeerCertificates
		if cl := len(certs); cl != 1 {
			conn.Close()
			return fmt.Errorf("unexpected certificate count: %d", cl)
		}

		remoteID := syncthingprotocol.NewDeviceID(certs[0].Raw)
		if remoteID != relayID {
			conn.Close()
			return fmt.Errorf("relay id does not match. Expected %v got %v", relayID, remoteID)
		}
	}

	return nil
}
