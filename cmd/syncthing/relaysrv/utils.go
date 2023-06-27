// Copyright (C) 2015 Audrius Butkevicius and Contributors.

package relaysrv

import (
	"errors"
	"net"
	"time"
)

func setTCPOptions(conn net.Conn, networkTimeout time.Duration) error {
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
	if err := tcpConn.SetKeepAlivePeriod(networkTimeout); err != nil {
		return err
	}
	if err := tcpConn.SetKeepAlive(true); err != nil {
		return err
	}
	return nil
}
