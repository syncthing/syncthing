// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package client

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	syncthingprotocol "github.com/syncthing/protocol"
	"github.com/syncthing/relaysrv/protocol"
)

func GetInvitationFromRelay(uri *url.URL, id syncthingprotocol.DeviceID, certs []tls.Certificate) (protocol.SessionInvitation, error) {
	if uri.Scheme != "relay" {
		return protocol.SessionInvitation{}, fmt.Errorf("Unsupported relay scheme:", uri.Scheme)
	}

	conn, err := tls.Dial("tcp", uri.Host, configForCerts(certs))
	if err != nil {
		return protocol.SessionInvitation{}, err
	}
	conn.SetDeadline(time.Now().Add(10 * time.Second))

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
		return protocol.SessionInvitation{}, fmt.Errorf("Incorrect response code %d: %s", msg.Code, msg.Message)
	case protocol.SessionInvitation:
		if debug {
			l.Debugln("Received invitation", msg, "via", conn.LocalAddr())
		}
		ip := net.IP(msg.Address)
		if len(ip) == 0 || ip.IsUnspecified() {
			msg.Address = conn.RemoteAddr().(*net.TCPAddr).IP[:]
		}
		return msg, nil
	default:
		return protocol.SessionInvitation{}, fmt.Errorf("protocol error: unexpected message %v", msg)
	}
}

func JoinSession(invitation protocol.SessionInvitation) (net.Conn, error) {
	addr := net.JoinHostPort(net.IP(invitation.Address).String(), strconv.Itoa(int(invitation.Port)))

	conn, err := net.Dial("tcp", addr)
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
			return nil, fmt.Errorf("Incorrect response code %d: %s", msg.Code, msg.Message)
		}
		return conn, nil
	default:
		return nil, fmt.Errorf("protocol error: expecting response got %v", msg)
	}
}

func TestRelay(uri *url.URL, certs []tls.Certificate, sleep time.Duration, times int) bool {
	id := syncthingprotocol.NewDeviceID(certs[0].Certificate[0])
	invs := make(chan protocol.SessionInvitation, 1)
	c := NewProtocolClient(uri, certs, invs)
	go c.Serve()
	defer func() {
		close(invs)
		c.Stop()
	}()

	for i := 0; i < times; i++ {
		_, err := GetInvitationFromRelay(uri, id, certs)
		if err == nil {
			return true
		}
		if !strings.Contains(err.Error(), "Incorrect response code") {
			return false
		}
		time.Sleep(sleep)
	}
	return false
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
