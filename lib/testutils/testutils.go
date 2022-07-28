// Copyright (C) 2019 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package testutils

import (
	"errors"
	"sync"
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

func (*NoopRW) Read(p []byte) (n int, err error) {
	return len(p), nil
}

func (*NoopRW) Write(p []byte) (n int, err error) {
	return len(p), nil
}

type NoopCloser struct{}

func (NoopCloser) Close() error {
	return nil
}
