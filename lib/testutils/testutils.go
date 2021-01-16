// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package testutils

import (
	"errors"
	"net"
	"sync"
	"time"
)

var ErrClosed = errors.New("closed")

// BlockingRW implements io.Reader, Writer and Closer, but only returns when closed
type BlockingRW struct {
	c         chan struct{}
	closeOnce sync.Once
}

func NewBlockingRW() *BlockingRW {
	return &BlockingRW{
		c:         make(chan struct{}),
		closeOnce: sync.Once{},
	}
}
func (rw *BlockingRW) Read(p []byte) (int, error) {
	<-rw.c
	return 0, ErrClosed
}

func (rw *BlockingRW) Write(p []byte) (int, error) {
	<-rw.c
	return 0, ErrClosed
}

func (rw *BlockingRW) Close() error {
	rw.closeOnce.Do(func() {
		close(rw.c)
	})
	return nil
}

// NoopRW implements io.Reader and Writer but never returns when called
type NoopRW struct{}

func (rw *NoopRW) Read(p []byte) (n int, err error) {
	return len(p), nil
}

func (rw *NoopRW) Write(p []byte) (n int, err error) {
	return len(p), nil
}

type NoopCloser struct{}

func (NoopCloser) Close() error {
	return nil
}

// FakeConnectionInfo implements the methods of protocol.Connection that are
// not implemented by protocol.Connection
type FakeConnectionInfo struct {
	Name string
}

func (f *FakeConnectionInfo) RemoteAddr() net.Addr {
	return &FakeAddr{}
}

func (f *FakeConnectionInfo) Type() string {
	return "fake"
}

func (f *FakeConnectionInfo) Crypto() string {
	return "fake"
}

func (f *FakeConnectionInfo) Transport() string {
	return "fake"
}

func (f *FakeConnectionInfo) Priority() int {
	return 9000
}

func (f *FakeConnectionInfo) String() string {
	return ""
}

func (f *FakeConnectionInfo) EstablishedAt() time.Time {
	return time.Time{}
}

type FakeAddr struct{}

func (FakeAddr) Network() string {
	return "network"
}

func (FakeAddr) String() string {
	return "address"
}
