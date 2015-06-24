// Copyright (C) 2015 Audrius Butkevicius and Contributors (see the CONTRIBUTORS file).

package main

import (
	"errors"
	"net"
	"time"
)

func setTCPOptions(conn net.Conn) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return errors.New("Not a TCP connection")
	}
	if err := tcpConn.SetLinger(0); err != nil {
		return err
	}
	if err := tcpConn.SetNoDelay(true); err != nil {
		return err
	}
	if err := tcpConn.SetKeepAlivePeriod(60 * time.Second); err != nil {
		return err
	}
	if err := tcpConn.SetKeepAlive(true); err != nil {
		return err
	}
	return nil
}

func sendMessage(msg message, conn net.Conn) error {
	header, err := msg.header.MarshalXDR()
	if err != nil {
		return err
	}

	err = conn.SetWriteDeadline(time.Now().Add(networkTimeout))
	if err != nil {
		return err
	}

	_, err = conn.Write(header)
	if err != nil {
		return err
	}

	_, err = conn.Write(msg.payload)
	if err != nil {
		return err
	}

	return nil
}
