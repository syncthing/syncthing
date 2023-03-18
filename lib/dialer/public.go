// Copyright (C) 2015 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package dialer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/syncthing/syncthing/lib/connections/registry"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.org/x/net/proxy"
)

var errUnexpectedInterfaceType = errors.New("unexpected interface type")

// SetTCPOptions sets our default TCP options on a TCP connection, possibly
// digging through dialerConn to extract the *net.TCPConn
func SetTCPOptions(conn net.Conn) error {
	switch conn := conn.(type) {
	case dialerConn:
		return SetTCPOptions(conn.Conn)
	case *net.TCPConn:
		var err error
		if err = conn.SetLinger(0); err != nil {
			return err
		}
		if err = conn.SetNoDelay(false); err != nil {
			return err
		}
		if err = conn.SetKeepAlivePeriod(60 * time.Second); err != nil {
			return err
		}
		if err = conn.SetKeepAlive(true); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown connection type %T", conn)
	}
}

func SetTrafficClass(conn net.Conn, class int) error {
	switch conn := conn.(type) {
	case dialerConn:
		return SetTrafficClass(conn.Conn, class)
	case *net.TCPConn:
		e1 := ipv4.NewConn(conn).SetTOS(class)
		e2 := ipv6.NewConn(conn).SetTrafficClass(class)

		if e1 != nil {
			return e1
		}
		return e2
	default:
		return fmt.Errorf("unknown connection type %T", conn)
	}
}

func dialContextWithFallback(ctx context.Context, fallback proxy.ContextDialer, network, addr string) (net.Conn, error) {
	dialer, ok := proxy.FromEnvironment().(proxy.ContextDialer)
	if !ok {
		return nil, errUnexpectedInterfaceType
	}
	if dialer == proxy.Direct {
		conn, err := fallback.DialContext(ctx, network, addr)
		l.Debugf("Dialing direct result %s %s: %v %v", network, addr, conn, err)
		return conn, err
	}
	if noFallback {
		conn, err := dialer.DialContext(ctx, network, addr)
		l.Debugf("Dialing no fallback result %s %s: %v %v", network, addr, conn, err)
		if err != nil {
			return nil, err
		}
		return dialerConn{conn, newDialerAddr(network, addr)}, nil
	}

	proxyDialFudgeAddress := func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return dialerConn{conn, newDialerAddr(network, addr)}, err
	}

	return dialTwicePreferFirst(ctx, proxyDialFudgeAddress, fallback.DialContext, "proxy", "fallback", network, addr)
}

// DialContext dials via context and/or directly, depending on how it is configured.
// If dialing via proxy and allowing fallback, dialing for both happens simultaneously
// and the proxy connection is returned if successful.
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return dialContextWithFallback(ctx, proxy.Direct, network, addr)
}

// DialContextReusePort tries dialing via proxy if a proxy is configured, and falls back to
// a direct connection reusing the port from the connections registry, if no proxy is defined, or connecting via proxy
// fails. It also in parallel dials without reusing the port, just in case reusing the port affects routing decisions badly.
func DialContextReusePortFunc(registry *registry.Registry) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// If proxy is configured, there is no point trying to reuse listen addresses.
		if proxy.FromEnvironment() != proxy.Direct {
			return DialContext(ctx, network, addr)
		}

		localAddrInterface := registry.Get(network, func(addr interface{}) bool {
			return addr.(*net.TCPAddr).IP.IsUnspecified()
		})
		if localAddrInterface == nil {
			// Nothing listening, nothing to reuse.
			return DialContext(ctx, network, addr)
		}

		laddr, ok := localAddrInterface.(*net.TCPAddr)
		if !ok {
			return nil, errUnexpectedInterfaceType
		}

		// Dial twice, once reusing the listen address, another time not reusing it, just in case reusing the address
		// influences routing and we fail to reach our destination.
		dialer := net.Dialer{
			Control:   ReusePortControl,
			LocalAddr: laddr,
		}
		return dialTwicePreferFirst(ctx, dialer.DialContext, (&net.Dialer{}).DialContext, "reuse", "non-reuse", network, addr)
	}
}

type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

func dialTwicePreferFirst(ctx context.Context, first, second dialFunc, firstName, secondName, network, address string) (net.Conn, error) {
	// Delay second dial by some time.
	sleep := time.Second
	if deadline, ok := ctx.Deadline(); ok {
		timeout := time.Until(deadline)
		if timeout > 0 {
			sleep = timeout / 3
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var firstConn, secondConn net.Conn
	var firstErr, secondErr error
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		firstConn, firstErr = first(ctx, network, address)
		l.Debugf("Dialing %s result %s %s: %v %v", firstName, network, address, firstConn, firstErr)
		close(firstDone)
	}()
	go func() {
		select {
		case <-firstDone:
			if firstErr == nil {
				// First succeeded, no point doing anything in second
				secondErr = errors.New("didn't dial")
				close(secondDone)
				return
			}
		case <-ctx.Done():
			secondErr = ctx.Err()
			close(secondDone)
			return
		case <-time.After(sleep):
		}
		secondConn, secondErr = second(ctx, network, address)
		l.Debugf("Dialing %s result %s %s: %v %v", secondName, network, address, secondConn, secondErr)
		close(secondDone)
	}()
	<-firstDone
	if firstErr == nil {
		go func() {
			<-secondDone
			if secondConn != nil {
				_ = secondConn.Close()
			}
		}()
		return firstConn, firstErr
	}
	<-secondDone
	return secondConn, secondErr
}
