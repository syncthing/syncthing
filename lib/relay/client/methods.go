// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/syncthing/syncthing/lib/dialer"
	syncthingprotocol "github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/relay/protocol"
)

type incorrectResponseCodeErr struct {
	code int32
	msg  string
}

func (e *incorrectResponseCodeErr) Error() string {
	return fmt.Sprintf("incorrect response code %d: %s", e.code, e.msg)
}

func GetInvitationFromRelay(ctx context.Context, uri *url.URL, id syncthingprotocol.DeviceID, certs []tls.Certificate, timeout time.Duration) (protocol.SessionInvitation, error) {
	if uri.Scheme != "relay" {
		return protocol.SessionInvitation{}, fmt.Errorf("unsupported relay scheme: %v", uri.Scheme)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	rconn, err := dialer.DialContext(ctx, "tcp", uri.Host)
	if err != nil {
		return protocol.SessionInvitation{}, err
	}

	conn := tls.Client(rconn, configForCerts(certs))
	conn.SetDeadline(time.Now().Add(timeout))

	if err := performHandshakeAndValidation(conn, uri); err != nil {
		return protocol.SessionInvitation{}, err
	}

	defer conn.Close()

	request := protocol.ConnectRequest{
		ID: id[:],
	}

	if err := protocol.WriteMessage(conn, request); err != nil {
		return protocol.SessionInvitation{}, err
	}

	message, err := protocol.ReadMessage(conn)
	if err != nil {
		return protocol.SessionInvitation{}, err
	}

	switch msg := message.(type) {
	case protocol.Response:
		return protocol.SessionInvitation{}, &incorrectResponseCodeErr{msg.Code, msg.Message}
	case protocol.SessionInvitation:
		l.Debugln("Received invitation", msg, "via", conn.LocalAddr())
		ip := net.IP(msg.Address)
		if len(ip) == 0 || ip.IsUnspecified() {
			msg.Address = remoteIPBytes(conn)
		}
		return msg, nil
	default:
		return protocol.SessionInvitation{}, fmt.Errorf("protocol error: unexpected message %v", msg)
	}
}

func JoinSession(ctx context.Context, invitation protocol.SessionInvitation) (net.Conn, error) {
	addr := net.JoinHostPort(net.IP(invitation.Address).String(), strconv.Itoa(int(invitation.Port)))

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	request := protocol.JoinSessionRequest{
		Key: invitation.Key,
	}

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	err = protocol.WriteMessage(conn, request)
	if err != nil {
		return nil, err
	}

	message, err := protocol.ReadMessage(conn)
	if err != nil {
		return nil, err
	}

	conn.SetDeadline(time.Time{})

	switch msg := message.(type) {
	case protocol.Response:
		if msg.Code != 0 {
			return nil, fmt.Errorf("incorrect response code %d: %s", msg.Code, msg.Message)
		}
		return conn, nil
	default:
		return nil, fmt.Errorf("protocol error: expecting response got %v", msg)
	}
}

func TestRelay(ctx context.Context, uri *url.URL, certs []tls.Certificate, sleep, timeout time.Duration, times int) error {
	id := syncthingprotocol.NewDeviceID(certs[0].Certificate[0])
	invs := make(chan protocol.SessionInvitation, 1)
	c, err := NewClient(uri, certs, invs, timeout)
	if err != nil {
		close(invs)
		return fmt.Errorf("creating client: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c.Serve(ctx)
		close(invs)
	}()
	defer cancel()

	for i := 0; i < times; i++ {
		_, err = GetInvitationFromRelay(ctx, uri, id, certs, timeout)
		if err == nil {
			return nil
		}
		if _, ok := err.(*incorrectResponseCodeErr); !ok {
			return fmt.Errorf("getting invitation: %w", err)
		}
		time.Sleep(sleep)
	}

	return fmt.Errorf("getting invitation: %w", err) // last of the above errors
}

func configForCerts(certs []tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates:           certs,
		NextProtos:             []string{protocol.ProtocolName},
		ClientAuth:             tls.RequestClientCert,
		SessionTicketsDisabled: true,
		InsecureSkipVerify:     true,
		MinVersion:             tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		},
	}
}

func remoteIPBytes(conn net.Conn) []byte {
	addr := conn.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	return net.ParseIP(addr)[:]
}
